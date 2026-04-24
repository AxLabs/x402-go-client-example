// Package main is the entry point for the x402-go-client-example CLI.
package main

import (
	"fmt"
	"os"

	"github.com/bane-labs-org/x402-go-client-example/internal/cli"
)

func main() {
	app := cli.NewApp()

	if err := app.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
