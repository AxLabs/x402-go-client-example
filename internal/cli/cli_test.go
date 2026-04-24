package cli

import (
	"os"
	"testing"
)

func TestNewApp(t *testing.T) {
	if NewApp() == nil {
		t.Fatal("NewApp() returned nil")
	}
}

func TestBuildRootCommand(t *testing.T) {
	cmd := NewApp().buildRootCommand()
	if cmd == nil {
		t.Fatal("buildRootCommand() returned nil")
	}
	if cmd.Use != "x402-client" {
		t.Errorf("Use = %q, want x402-client", cmd.Use)
	}
	for _, name := range []string{"get", "post", "inspect", "version"} {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Use == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestBuildGetCommand_Flags(t *testing.T) {
	cmd := NewApp().buildGetCommand()
	for _, flag := range []string{"url", "dry-run", "no-pay"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}

func TestBuildPostCommand_Flags(t *testing.T) {
	cmd := NewApp().buildPostCommand()
	for _, flag := range []string{"url", "body", "dry-run", "no-pay"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}

func TestSetup(t *testing.T) {
	os.Unsetenv("CLIENT_LOG_LEVEL")
	os.Unsetenv("CLIENT_TIMEOUT")

	a := NewApp()
	cmd := a.buildRootCommand()
	if err := a.setup(cmd, nil); err != nil {
		t.Fatalf("setup() err = %v", err)
	}
	if a.config == nil || a.logger == nil {
		t.Fatal("setup() must populate config + logger")
	}
}
package cli

import (
	"os"
	"testing"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp() returned nil")
	}
}

func TestBuildRootCommand(t *testing.T) {
	app := NewApp()
	cmd := app.buildRootCommand()

	if cmd == nil {
		t.Fatal("buildRootCommand() returned nil")
	}

	if cmd.Use != "x402-client" {
		t.Errorf("Root command Use = %q, want %q", cmd.Use, "x402-client")
	}

	// Check that subcommands exist
	subcommands := []string{"get", "post", "inspect", "version"}
	for _, name := range subcommands {
		found := false
		for _, subcmd := range cmd.Commands() {
			if subcmd.Use == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand %q not found", name)
		}
	}
}

func TestBuildGetCommand(t *testing.T) {
	app := NewApp()
	cmd := app.buildGetCommand()

	if cmd == nil {
		t.Fatal("buildGetCommand() returned nil")
	}

	if cmd.Use != "get" {
		t.Errorf("Get command Use = %q, want %q", cmd.Use, "get")
	}

	// Check flags exist
	flags := []string{"url", "dry-run", "no-pay"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag %q not found", flag)
		}
	}
}

func TestBuildPostCommand(t *testing.T) {
	app := NewApp()
	cmd := app.buildPostCommand()

	if cmd == nil {
		t.Fatal("buildPostCommand() returned nil")
	}

	if cmd.Use != "post" {
		t.Errorf("Post command Use = %q, want %q", cmd.Use, "post")
	}

	// Check flags exist
	flags := []string{"url", "body", "dry-run", "no-pay"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag %q not found", flag)
		}
	}
}

func TestBuildInspectCommand(t *testing.T) {
	app := NewApp()
	cmd := app.buildInspectCommand()

	if cmd == nil {
		t.Fatal("buildInspectCommand() returned nil")
	}

	if cmd.Use != "inspect" {
		t.Errorf("Inspect command Use = %q, want %q", cmd.Use, "inspect")
	}

	// Check URL flag exists
	if cmd.Flags().Lookup("url") == nil {
		t.Error("Expected flag 'url' not found")
	}
}

func TestBuildVersionCommand(t *testing.T) {
	app := NewApp()
	cmd := app.buildVersionCommand()

	if cmd == nil {
		t.Fatal("buildVersionCommand() returned nil")
	}

	if cmd.Use != "version" {
		t.Errorf("Version command Use = %q, want %q", cmd.Use, "version")
	}
}

func TestSetup(t *testing.T) {
	// Clear any conflicting env vars
	os.Unsetenv("CLIENT_LOG_LEVEL")
	os.Unsetenv("CLIENT_TIMEOUT")

	app := NewApp()
	cmd := app.buildRootCommand()

	err := app.setup(cmd, nil)
	if err != nil {
		t.Fatalf("setup() error = %v", err)
	}

	if app.config == nil {
		t.Error("config should not be nil after setup")
	}

	if app.logger == nil {
		t.Error("logger should not be nil after setup")
	}
}

func TestSetupVerbose(t *testing.T) {
	// Skip this test in short mode as it depends on cobra command parsing
	if testing.Short() {
		t.Skip("skipping verbose setup test in short mode")
	}

	os.Unsetenv("CLIENT_LOG_LEVEL")

	app := NewApp()
	app.verbose = true

	cmd := app.buildRootCommand()
	err := app.setup(cmd, nil)
	if err != nil {
		t.Fatalf("setup() error = %v", err)
	}

	// Verify that setup completed successfully with verbose mode
	// The actual verbose behavior is tested via CLI integration tests
	if app.config == nil {
		t.Error("config should not be nil after setup")
	}
	if app.logger == nil {
		t.Error("logger should not be nil after setup")
	}
}
