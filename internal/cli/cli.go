// Package cli implements the command-line interface for the x402 client example.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/bane-labs-org/x402-go-client-example/internal/config"
	"github.com/bane-labs-org/x402-go-client-example/internal/httpclient"
	"github.com/bane-labs-org/x402-go-client-example/internal/logging"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/signer"
	"github.com/bane-labs-org/x402-go-client-example/internal/version"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// App holds the CLI application state.
type App struct {
	config *config.Config
	logger *logging.Logger

	url     string
	body    string
	dryRun  bool
	noPay   bool
	verbose bool
	timeout time.Duration
}

// NewApp creates a new CLI application.
func NewApp() *App { return &App{} }

// Execute runs the CLI application.
func (a *App) Execute() error { return a.buildRootCommand().Execute() }

func (a *App) buildRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "x402-client",
		Short: "x402 Payment Protocol Client Example (powered by the official x402 Go SDK)",
		Long: `A buyer/caller for the x402 payment flow built on top of the official
x402 Go SDK (github.com/x402-foundation/x402/go).

This tool only orchestrates the flow:
  - calls a protected endpoint
  - on HTTP 402, asks the SDK to parse the response
  - validates the selected requirements against local policy
  - asks the SDK to build a signed payment payload (EVM exact scheme)
  - retries the request with SDK-produced headers

All protocol concerns (parsing, signing, wire formats, header encoding)
live in the SDK.`,
		PersistentPreRunE: a.setup,
		SilenceUsage:      true,
	}

	rootCmd.PersistentFlags().BoolVarP(&a.verbose, "verbose", "v", false, "Enable verbose/debug output")
	rootCmd.PersistentFlags().DurationVar(&a.timeout, "timeout", 30*time.Second, "Request timeout")

	rootCmd.AddCommand(a.buildGetCommand())
	rootCmd.AddCommand(a.buildPostCommand())
	rootCmd.AddCommand(a.buildInspectCommand())
	rootCmd.AddCommand(a.buildVersionCommand())

	return rootCmd
}

func (a *App) setup(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if a.verbose {
		cfg.LogLevel = "debug"
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	a.config = cfg
	a.logger = logging.New(logging.Options{
		Level:  logging.ParseLevel(cfg.LogLevel),
		Output: os.Stderr,
		JSON:   cfg.LogJSON,
	})
	return nil
}

func (a *App) buildGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Perform a GET request with payment flow handling",
		Long: `Perform a GET request to a protected endpoint.

On HTTP 402 the SDK parses the response and builds a signed payment payload
which is retried back on the same URL.`,
		RunE: a.runGet,
	}
	cmd.Flags().StringVar(&a.url, "url", "", "Target URL (required)")
	cmd.Flags().BoolVar(&a.dryRun, "dry-run", false, "Parse 402 but don't sign or retry")
	cmd.Flags().BoolVar(&a.noPay, "no-pay", false, "Don't attempt payment flow")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func (a *App) buildPostCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post",
		Short: "Perform a POST request with payment flow handling",
		RunE:  a.runPost,
	}
	cmd.Flags().StringVar(&a.url, "url", "", "Target URL (required)")
	cmd.Flags().StringVar(&a.body, "body", "", "Request body (JSON)")
	cmd.Flags().BoolVar(&a.dryRun, "dry-run", false, "Parse 402 but don't sign or retry")
	cmd.Flags().BoolVar(&a.noPay, "no-pay", false, "Don't attempt payment flow")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func (a *App) buildInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Fetch and display 402 payment requirements without paying",
		RunE:  a.runInspect,
	}
	cmd.Flags().StringVar(&a.url, "url", "", "Target URL (required)")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func (a *App) buildVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Get().String())
		},
	}
}

func (a *App) runGet(cmd *cobra.Command, args []string) error {
	return a.executeRequest("GET", a.url, nil)
}

func (a *App) runPost(cmd *cobra.Command, args []string) error {
	var body []byte
	if a.body != "" {
		body = []byte(a.body)
	}
	return a.executeRequest("POST", a.url, body)
}

func (a *App) runInspect(cmd *cobra.Command, args []string) error {
	a.noPay = true
	return a.executeRequest("GET", a.url, nil)
}

func (a *App) executeRequest(method, url string, body []byte) error {
	a.printStep("Starting x402 payment flow")
	a.printInfo("Method", method)
	a.printInfo("URL", url)

	// Build signer via the SDK-backed wrapper (if a private key is configured).
	var s signer.Signer
	if a.config.HasPrivateKey() {
		es, err := signer.NewEthereumSigner(a.config.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to create signer: %w", err)
		}
		s = es
		a.printInfo("Signer Address", s.Address())
	} else {
		a.printWarning("No private key configured - payment signing disabled")
	}

	// Build the x402 adapter. Registering the signer enables the EVM exact
	// scheme; with a nil signer we can still parse 402 responses.
	var sdkSigner x402adapter.EVMSigner
	if s != nil {
		sdkSigner = s.EVMSigner()
	}
	adapter := x402adapter.NewForEVM(sdkSigner)

	// Build local policy.
	allowedChains := []string{}
	if a.config.AllowedChainID != "" {
		allowedChains = []string{a.config.AllowedChainID}
	}
	pol := policy.NewPolicyFromConfig(
		a.config.GetMaxAmountUint64(),
		a.config.AllowedAssets,
		allowedChains,
		a.config.AllowedPayTo,
	)
	a.printInfo("Policy", pol.String())

	// Build orchestrator.
	client := httpclient.New(httpclient.Options{
		Timeout: a.timeout,
		Adapter: adapter,
		Policy:  pol,
		Logger:  a.logger,
		DryRun:  a.dryRun || a.config.DryRun,
		NoPay:   a.noPay || a.config.NoPay,
	})

	a.printStep("Making initial request")
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	var result *httpclient.RequestResult
	var err error
	if method == "POST" {
		result, err = client.Post(ctx, url, body)
	} else {
		result, err = client.Get(ctx, url)
	}
	if err != nil {
		a.printError("Request failed", err.Error())
		return err
	}

	a.displayResult(result)

	if result.Response.StatusCode >= 400 && !result.PaymentMade {
		if result.PaymentRequired {
			if a.dryRun {
				a.printInfo("Dry-run mode", "Payment not attempted")
				return nil
			}
			if a.noPay {
				a.printInfo("No-pay mode", "Payment not attempted")
				return nil
			}
		}
		return fmt.Errorf("request failed with status %d", result.Response.StatusCode)
	}

	return nil
}

func (a *App) displayResult(result *httpclient.RequestResult) {
	fmt.Println()

	if result.PaymentRequired {
		a.printStep("Received 402 Payment Required")
		if result.Requirements != nil {
			fmt.Println(httpclient.FormatRequirements(result.Requirements))
		}
	}

	if result.PaymentMade {
		a.printStep("Payment authorized and retry completed (SDK-signed)")
		if result.PaymentPayload != nil {
			if j, err := json.MarshalIndent(result.PaymentPayload, "    ", "  "); err == nil {
				fmt.Printf("    Payment Payload:\n    %s\n", string(j))
			}
		}
	}

	a.printStep("Final Response")
	a.printInfo("Status", fmt.Sprintf("%d %s", result.Response.StatusCode, result.Response.Status))
	a.printInfo("Content-Type", result.Response.Header.Get("Content-Type"))

	fmt.Println()
	fmt.Println("Body:")

	var pretty map[string]interface{}
	if err := json.Unmarshal(result.Body, &pretty); err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pretty)
	} else {
		fmt.Println(string(result.Body))
	}
}

func (a *App) printStep(msg string)         { fmt.Printf("\n==> %s\n", msg) }
func (a *App) printInfo(key, value string)  { fmt.Printf("    %s: %s\n", key, value) }
func (a *App) printWarning(msg string)      { fmt.Printf("    [WARNING] %s\n", msg) }
func (a *App) printError(title, msg string) { fmt.Printf("\n[ERROR] %s: %s\n", title, msg) }
