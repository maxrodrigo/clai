// Package datadir resolves the CLAI data directory containing prompts and strategies.
package datadir

import (
	"os"
	"path/filepath"
)

// DefaultDataDir can be set at build time via ldflags:
//
//	-ldflags "-X github.com/maxrodrigo/clai/internal/datadir.DefaultDataDir=/usr/local/share/clai"
var DefaultDataDir string

// Dir returns the CLAI data directory using a 4-step resolution:
//  1. $CLAI_DATA_DIR environment variable (explicit override)
//  2. Binary-relative path: <binary>/../share/clai (works with Homebrew Cellar layout)
//  3. XDG user data: $XDG_DATA_HOME/clai, defaulting to ~/.local/share/clai (make install default)
//  4. Compiled-in DefaultDataDir (set via ldflags at build time)
//
// Returns empty string if no valid data directory is found.
func Dir() string {
	// 1. Environment variable takes precedence
	if dir := os.Getenv("CLAI_DATA_DIR"); dir != "" {
		return dir
	}

	// 2. Binary-relative: resolve symlinks then look for ../share/clai
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			candidate := filepath.Join(filepath.Dir(resolved), "..", "share", "clai")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
	}

	// 3. XDG user data directory ($XDG_DATA_HOME, or ~/.local/share by spec)
	if base := xdgDataHome(); base != "" {
		candidate := filepath.Join(base, "clai")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	// 4. Compiled-in default (set via ldflags)
	return DefaultDataDir
}

// xdgDataHome returns the XDG base data directory, honoring $XDG_DATA_HOME and
// falling back to ~/.local/share per the XDG Base Directory specification.
// Returns "" if the home directory cannot be determined.
func xdgDataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}
