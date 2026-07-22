// Package output handles terminal output, formatting, and TTY detection.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"golang.org/x/term"
)

const (
	// spinnerInterval is the animation frame rate for the spinner.
	spinnerInterval = 80 * time.Millisecond
	// truncateMarker is appended when content is truncated.
	truncateMarker = "\n[...truncated]"
)

// Shared color palette. All colored output in the application flows through
// these variables so the scheme can be changed in one place.
var (
	colorError   = color.New(color.FgRed)
	colorWarning = color.New(color.FgYellow)
	colorSuccess = color.New(color.FgGreen)
	colorDiag    = color.New(color.Faint)
	colorInfo    = color.New(color.FgCyan)
)

// Output handles writing to stdout and stderr with formatting support.
type Output struct {
	Stdout io.Writer
	Stderr io.Writer
}

// New returns an Output wired to the standard OS file descriptors.
func New() *Output {
	return &Output{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// IsStdoutTerminal reports whether Stdout is an interactive terminal.
func (o *Output) IsStdoutTerminal() bool {
	return isWriterTerminal(o.Stdout)
}

// IsStderrTerminal reports whether Stderr is an interactive terminal.
func (o *Output) IsStderrTerminal() bool {
	return isWriterTerminal(o.Stderr)
}

// isWriterTerminal reports whether w is backed by an interactive terminal.
// Returns false for any writer that is not an *os.File (e.g. a *bytes.Buffer
// in tests), which is the correct and desired behaviour.
func isWriterTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// estimateTokens returns a rough token count estimate for text.
// Uses the approximation of 1 token per 4 characters, which is reasonable
// across most English-language models.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}

// DryRunMessage is one replayed conversation turn for dry-run display.
type DryRunMessage struct {
	Role    string
	Content string
}

// PrintDryRunHistory writes replayed conversation history to stderr, one line
// per turn, truncated for readability. Called before PrintDryRun when a
// conversation is active.
func (o *Output) PrintDryRunHistory(msgs []DryRunMessage) {
	for _, m := range msgs {
		_, _ = colorDiag.Fprintf(o.Stderr, "[clai] history %s: %s\n", m.Role, truncateRunes([]byte(m.Content), 100))
	}
}

// PrintDryRun prints what would be sent to the model without calling it.
// Includes model, prompts, and estimated token counts.
func (o *Output) PrintDryRun(model, systemPrompt, userMessage string) {
	estimatedIn := estimateTokens(systemPrompt) + estimateTokens(userMessage)

	_, _ = colorInfo.Fprintf(o.Stderr, "[clai] dry-run: would send to %s\n", model)
	if systemPrompt != "" {
		_, _ = colorDiag.Fprintf(o.Stderr, "[clai] system: %s\n", truncateRunes([]byte(systemPrompt), 200))
	}
	_, _ = colorDiag.Fprintf(o.Stderr, "[clai] user: %s\n", truncateRunes([]byte(userMessage), 200))
	_, _ = colorDiag.Fprintf(o.Stderr, "[clai] ~tokens_in=%d\n", estimatedIn)
}

// PrintVerbosePre writes pre-call query details to stderr.
// Called before the model call when --verbose is set.
func (o *Output) PrintVerbosePre(model, systemPrompt, userMessage string) {
	estimatedIn := estimateTokens(systemPrompt) + estimateTokens(userMessage)
	_, _ = colorDiag.Fprintf(o.Stderr, "[clai] model=%s ~tokens_in=%d\n", model, estimatedIn)
}

// PrintVerbosePost writes token usage and timing to stderr after a model call.
func (o *Output) PrintVerbosePost(inputTokens, outputTokens int, elapsed float64) {
	_, _ = colorDiag.Fprintf(o.Stderr, "[clai] in=%d out=%d total=%d time=%.2fs\n",
		inputTokens, outputTokens,
		inputTokens+outputTokens,
		elapsed)
}

// WriteOutput writes content to stdout. If pretty is true and content is
// valid JSON, it is pretty-printed. A trailing newline is always added if the
// content does not already end with one.
func (o *Output) WriteOutput(content string, pretty bool) {
	if pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(content), "", "  "); err == nil {
			content = buf.String()
		}
	}
	_, _ = fmt.Fprint(o.Stdout, content)
	if content != "" && content[len(content)-1] != '\n' {
		_, _ = fmt.Fprintln(o.Stdout)
	}
}

// PrintError writes an error message to stderr in red.
func (o *Output) PrintError(format string, args ...any) {
	_, _ = colorError.Fprintf(o.Stderr, format, args...)
}

// PrintWarning writes a warning message to stderr in yellow.
func (o *Output) PrintWarning(format string, args ...any) {
	_, _ = colorWarning.Fprintf(o.Stderr, format, args...)
}

// PrintSuccess writes a success confirmation to stderr in green.
func (o *Output) PrintSuccess(format string, args ...any) {
	_, _ = colorSuccess.Fprintf(o.Stderr, format, args...)
}

// PrintHint writes a faint hint message to stderr.
func (o *Output) PrintHint(format string, args ...any) {
	_, _ = colorDiag.Fprintf(o.Stderr, format, args...)
}

// WarnWriter returns a writer that applies yellow warning color to all writes.
// Use this to pass a color-aware warning writer to packages that only accept
// an io.Writer (e.g. prompt, strategy) so their warnings are consistently styled.
func WarnWriter(w io.Writer) io.Writer {
	return &colorWriter{w: w, c: colorWarning}
}

// colorWriter is an io.Writer that applies a color attribute to every write.
type colorWriter struct {
	w io.Writer
	c *color.Color
}

func (cw *colorWriter) Write(p []byte) (int, error) {
	_, err := cw.c.Fprint(cw.w, string(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// truncateRunes truncates data to at most n bytes, cutting only at rune
// boundaries to avoid emitting invalid UTF-8 sequences.
func truncateRunes(data []byte, n int) []byte {
	if len(data) <= n {
		return data
	}
	// Walk back from byte n until we find a rune boundary.
	i := n
	for i > 0 && !utf8.RuneStart(data[i]) {
		i--
	}
	return append(data[:i], truncateMarker...)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated braille spinner on stderr while work is in
// progress. It is a no-op when stderr is not a TTY, so piped output is never
// polluted.
type Spinner struct {
	stderr io.Writer
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// NewSpinner starts a spinner with the given label and returns it.
func (o *Output) NewSpinner(label string) *Spinner {
	s := &Spinner{
		stderr: o.Stderr,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	if !o.IsStderrTerminal() {
		close(s.done) // no-op: Stop can wait on done safely
		return s
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ticker.C:
				_, _ = fmt.Fprintf(s.stderr, "\r%s %s", spinnerFrames[i%len(spinnerFrames)], label)
			case <-s.stop:
				_, _ = fmt.Fprintf(s.stderr, "\r\033[K")
				return
			}
		}
	}()
	return s
}

// Stop halts the spinner and clears the line. Safe to call multiple times.
func (s *Spinner) Stop() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

// SpinnerWriter wraps a Writer and stops the spinner on the first Write.
// Use it to pass to CompleteStream so the spinner clears the moment the
// first token arrives.
type SpinnerWriter struct {
	io.Writer
	spinner *Spinner
	once    sync.Once
}

// NewSpinnerWriter returns a SpinnerWriter that writes to w and stops s on
// the first Write call.
func NewSpinnerWriter(w io.Writer, s *Spinner) *SpinnerWriter {
	return &SpinnerWriter{Writer: w, spinner: s}
}

func (sw *SpinnerWriter) Write(p []byte) (int, error) {
	sw.once.Do(sw.spinner.Stop)
	return sw.Writer.Write(p)
}
