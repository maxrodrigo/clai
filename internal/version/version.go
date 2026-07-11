// Package version holds build-time version info.
package version

import (
	"runtime/debug"
)

// Version is set via ldflags at build time by GoReleaser or make:
//
//	go build -ldflags "-X github.com/maxrodrigo/clai/internal/version.Version=1.0.0"
//
// When not set (e.g., `go install ...@latest`), falls back to VCS info from
// the Go module build metadata.
var Version = ""

// DataDir is set at runtime by main() after resolving the data directory.
// Used for diagnostic output in --version.
var DataDir string

func init() {
	if Version != "" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		Version = "dev"
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
		return
	}
	Version = "dev"
}

// String returns the full version string including data directory if set.
func String() string {
	s := Version
	if DataDir != "" {
		s += " (data: " + DataDir + ")"
	} else {
		s += " (data: not configured)"
	}
	return s
}
