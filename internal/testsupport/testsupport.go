// Package testsupport provides helpers shared across clai's test suites.
//
// It exists so tests are hermetic: they locate the in-repo data directory
// (share/clai) by source position rather than depending on CLAI_DATA_DIR being
// set by the test runner, and they isolate the HOME and XDG environment so a
// developer's personal config under ~/.config/clai cannot leak into results.
package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the absolute path to the repository's share/clai directory,
// resolved relative to this source file. This keeps tests independent of the
// caller's working directory and of the CLAI_DATA_DIR environment variable.
//
// It is safe to call from TestMain (which has no *testing.T): runtime.Caller
// always resolves for a compiled source file.
func DataDir() string {
	_, file, _, _ := runtime.Caller(0)
	// file is .../internal/testsupport/testsupport.go; the repo root is two
	// levels above internal/.
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(repoRoot, "share", "clai")
}

// IsolateConfig points XDG_CONFIG_HOME at a fresh temporary directory so a
// developer's real ~/.config/clai cannot leak into a test run, and returns a
// cleanup function that removes it. It is meant for use in TestMain, where
// *testing.T (and thus t.Setenv) is unavailable.
func IsolateConfig() (cleanup func()) {
	dir, err := os.MkdirTemp("", "clai-test-config-")
	if err != nil {
		panic("testsupport: create temp config dir: " + err.Error())
	}
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		panic("testsupport: set XDG_CONFIG_HOME: " + err.Error())
	}
	return func() { _ = os.RemoveAll(dir) }
}
