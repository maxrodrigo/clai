// Package source handles input resolution for clai.
//
// Sources are file paths or stdin. Multiple file sources are resolved
// concurrently and joined in the order they were specified.
package source

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/maxrodrigo/clai/internal/config"
	"golang.org/x/term"
)

// maxInputBytes is a safety cap to prevent accidental OOM from unbounded reads.
// This is not a token-budget limit — it catches cases like piping a binary
// or infinite stream. 10 MB is well above any realistic text input for an LLM.
const maxInputBytes = 10 << 20 // 10 MB

// Input handles reading from stdin and resolving input sources.
type Input struct {
	Stdin  io.Reader
	Stderr io.Writer // for warnings (e.g., ContinueOnError)
}

// IsStdinTerminal reports whether Stdin is an interactive terminal.
func (in *Input) IsStdinTerminal() bool {
	f, ok := in.Stdin.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// Resolve reads and concatenates all input sources concurrently.
// Returns nil if no sources are given and stdin is a terminal.
func (in *Input) Resolve(paths []string, cfg *config.Config) ([]byte, error) {
	if len(paths) == 0 {
		return in.readStdin()
	}

	results := make([][]byte, len(paths))
	errs := make([]error, len(paths))

	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			data, err := resolveFile(path)
			results[i] = data
			errs[i] = err
		}(i, path)
	}
	wg.Wait()

	var parts [][]byte
	for i, err := range errs {
		if err != nil {
			if cfg.ContinueOnError {
				_, _ = fmt.Fprintf(in.Stderr, "warning: %s — skipping\n", err)
				continue
			}
			return nil, err
		}
		parts = append(parts, results[i])
	}

	if len(parts) == 0 {
		return nil, errors.New("all sources failed")
	}

	return bytes.Join(parts, []byte("\n\n")), nil
}

// resolveFile reads a file from disk with a safety size cap.
func resolveFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			return nil, fmt.Errorf("reading %s: %w", path, pathErr.Err)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	data, err := readLimited(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// readStdin reads from stdin if it is piped (not a terminal).
// Returns nil, nil if stdin is a terminal (interactive session).
func (in *Input) readStdin() ([]byte, error) {
	if in.IsStdinTerminal() {
		return nil, nil
	}
	return readLimited(in.Stdin)
}

// readLimited reads up to maxInputBytes from r. Returns an error if the input
// exceeds the safety cap.
func readLimited(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, maxInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("input too large (exceeds %d MB limit)", maxInputBytes>>20)
	}
	return data, nil
}
