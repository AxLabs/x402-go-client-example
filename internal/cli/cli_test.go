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
	if cmd == nil {
		t.Fatal("buildGetCommand() returned nil")
	}
	if cmd.Use != "get" {
		t.Errorf("Use = %q, want get", cmd.Use)
	}
	for _, flag := range []string{"url", "dry-run", "no-pay"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}

func TestBuildPostCommand_Flags(t *testing.T) {
	cmd := NewApp().buildPostCommand()
	if cmd == nil {
		t.Fatal("buildPostCommand() returned nil")
	}
	if cmd.Use != "post" {
		t.Errorf("Use = %q, want post", cmd.Use)
	}
	for _, flag := range []string{"url", "body", "dry-run", "no-pay"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}

func TestBuildInspectCommand(t *testing.T) {
	cmd := NewApp().buildInspectCommand()
	if cmd == nil {
		t.Fatal("buildInspectCommand() returned nil")
	}
	if cmd.Use != "inspect" {
		t.Errorf("Use = %q, want inspect", cmd.Use)
	}
	if cmd.Flags().Lookup("url") == nil {
		t.Error("missing flag 'url'")
	}
}

func TestBuildVersionCommand(t *testing.T) {
	cmd := NewApp().buildVersionCommand()
	if cmd == nil {
		t.Fatal("buildVersionCommand() returned nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want version", cmd.Use)
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

func TestSetupVerbose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping verbose setup test in short mode")
	}

	os.Unsetenv("CLIENT_LOG_LEVEL")

	a := NewApp()
	a.verbose = true
	cmd := a.buildRootCommand()
	if err := a.setup(cmd, nil); err != nil {
		t.Fatalf("setup() err = %v", err)
	}
	if a.config == nil || a.logger == nil {
		t.Fatal("setup() must populate config + logger")
	}
}
