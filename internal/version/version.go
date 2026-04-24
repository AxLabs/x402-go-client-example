// Package version provides build-time version information for the client.
package version

import (
	"fmt"
	"runtime"
)

// Build information set at compile time via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info holds complete version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current version information.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	return fmt.Sprintf(
		"x402-go-client-example %s (commit: %s, built: %s, %s %s/%s)",
		i.Version,
		i.Commit,
		i.BuildTime,
		i.GoVersion,
		i.OS,
		i.Arch,
	)
}

// Short returns a short version string.
func (i Info) Short() string {
	return fmt.Sprintf("x402-go-client-example %s", i.Version)
}
