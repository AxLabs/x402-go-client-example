// Package main is the entry point for the x402-buyer-client-go CLI.
package main

import (
	"fmt"
	"os"

	"github.com/bane-labs-org/x402-buyer-client-go/internal/cli"
)

func main() {
	app := cli.NewApp()

	if err := app.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
