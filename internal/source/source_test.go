package source

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/maxrodrigo/clai/internal/config"
)

func TestResolveStdin(t *testing.T) {
	in := &Input{
		Stdin:  strings.NewReader("hello from stdin"),
		Stderr: &bytes.Buffer{},
	}

	cfg := &config.Config{}
	data, err := in.Resolve(nil, cfg)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got, want := string(data), "hello from stdin"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolveFile(t *testing.T) {
	path := writeTempFile(t, "file content")
	in := &Input{Stdin: os.Stdin, Stderr: io.Discard}

	cfg := &config.Config{}
	paths := []string{path}
	data, err := in.Resolve(paths, cfg)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got, want := string(data), "file content"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolveMultipleFiles(t *testing.T) {
	a := writeTempFile(t, "aaa")
	b := writeTempFile(t, "bbb")
	in := &Input{Stdin: os.Stdin, Stderr: io.Discard}

	cfg := &config.Config{}
	paths := []string{a, b}
	data, err := in.Resolve(paths, cfg)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	// Order must be preserved despite concurrent resolution.
	if got, want := string(data), "aaa\n\nbbb"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolveMissingFile(t *testing.T) {
	in := &Input{Stdin: os.Stdin, Stderr: io.Discard}
	cfg := &config.Config{}
	paths := []string{"/nonexistent/file.txt"}
	_, err := in.Resolve(paths, cfg)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestResolveContinueOnError(t *testing.T) {
	good := writeTempFile(t, "good content")
	in := &Input{
		Stdin:  strings.NewReader(""),
		Stderr: &bytes.Buffer{},
	}

	cfg := &config.Config{ContinueOnError: true}
	paths := []string{"/nonexistent.txt", good}
	data, err := in.Resolve(paths, cfg)
	if err != nil {
		t.Fatalf("Resolve() with ContinueOnError should not fail: %v", err)
	}
	if got, want := string(data), "good content"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestIsStdinTerminal_buffer(t *testing.T) {
	in := &Input{
		Stdin:  strings.NewReader(""),
		Stderr: &bytes.Buffer{},
	}
	if in.IsStdinTerminal() {
		t.Error("IsStdinTerminal() should return false for *strings.Reader")
	}
}

func TestResolveContinueOnError_allFail(t *testing.T) {
	in := &Input{
		Stdin:  strings.NewReader(""),
		Stderr: &bytes.Buffer{},
	}
	cfg := &config.Config{ContinueOnError: true}
	paths := []string{"/nonexistent1.txt", "/nonexistent2.txt"}
	_, err := in.Resolve(paths, cfg)
	if err == nil {
		t.Fatal("Resolve() with all failing sources should error")
	}
	if !strings.Contains(err.Error(), "all sources failed") {
		t.Errorf("expected 'all sources failed' error, got: %v", err)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "source-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
