package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestTruncateRunes(t *testing.T) {
	t.Run("truncates long input and adds marker", func(t *testing.T) {
		data := []byte(strings.Repeat("a", 600))
		got := truncateRunes(data, 500)
		// Content is capped at 500 bytes, then the marker appended.
		if len(got) > 515 {
			t.Errorf("truncateRunes: result too long: %d bytes", len(got))
		}
		if !bytes.Contains(got, []byte("[...truncated]")) {
			t.Errorf("truncateRunes: missing truncation marker in %q", got)
		}
	})

	t.Run("short input passes through unchanged", func(t *testing.T) {
		short := []byte("hello")
		if got, want := string(truncateRunes(short, 500)), "hello"; got != want {
			t.Errorf("truncateRunes(short) = %q, want %q", got, want)
		}
	})

	t.Run("respects UTF-8 rune boundaries", func(t *testing.T) {
		// "é" is 2 bytes (0xC3 0xA9). Build a string where truncating at byte 501
		// would land mid-rune. We want the output to be valid UTF-8.
		base := strings.Repeat("é", 252) // 252 × 2 = 504 bytes
		data := []byte(base + strings.Repeat("x", 100))
		got := truncateRunes(data, 501) // 501 lands mid-rune
		// strings.ToValidUTF8 replaces invalid sequences; if it changes the string
		// then got contains invalid UTF-8.
		if strings.ToValidUTF8(string(got), "\uFFFD") != string(got) {
			t.Errorf("truncateRunes: result contains invalid UTF-8")
		}
	})
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 0},
		{"short text", "hi", 1},
		{"four chars", "test", 1},
		{"five chars", "hello", 2},
		{"typical sentence", "The quick brown fox jumps over the lazy dog.", 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.input)
			if got != tt.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrintDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &buf}

	o.PrintDryRun("ollama/llama3.2", "You are helpful.", "What is Go?")

	out := buf.String()
	if !strings.Contains(out, "[clai] dry-run:") {
		t.Errorf("PrintDryRun output missing [clai] dry-run: prefix: %q", out)
	}
	if !strings.Contains(out, "ollama/llama3.2") {
		t.Errorf("PrintDryRun output missing model: %q", out)
	}
	if !strings.Contains(out, "~tokens_in=") {
		t.Errorf("PrintDryRun output missing token estimate: %q", out)
	}
}

func TestSpinner_noop_when_not_tty(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &buf}

	s := o.NewSpinner("working")
	s.Stop()

	if buf.Len() != 0 {
		t.Errorf("Spinner wrote to non-TTY stderr: %q", buf.String())
	}
}

func TestSpinner_stop_is_idempotent(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &buf}

	s := o.NewSpinner("working")
	s.Stop()
	s.Stop() // must not panic or deadlock
}

func TestWriteOutput(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &buf, Stderr: &bytes.Buffer{}}

	o.WriteOutput("hello", false)
	if got, want := buf.String(), "hello\n"; got != want {
		t.Errorf("WriteOutput() = %q, want %q", got, want)
	}
}

func TestWriteOutput_withNewline(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &buf, Stderr: &bytes.Buffer{}}

	o.WriteOutput("hello\n", false)
	if got, want := buf.String(), "hello\n"; got != want {
		t.Errorf("WriteOutput() = %q, want %q (should not double newline)", got, want)
	}
}

func TestWriteOutput_prettyJSON(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &buf, Stderr: &bytes.Buffer{}}

	o.WriteOutput(`{"a":1}`, true)
	out := buf.String()
	if !strings.Contains(out, "\"a\": 1") {
		t.Errorf("WriteOutput(pretty=true) should indent JSON, got %q", out)
	}
}

func TestNew(t *testing.T) {
	o := New()
	if o.Stdout == nil || o.Stderr == nil {
		t.Error("New() should return non-nil Stdout and Stderr")
	}
}

func TestIsStdoutTerminal_buffer(t *testing.T) {
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if o.IsStdoutTerminal() {
		t.Error("IsStdoutTerminal() should return false for *bytes.Buffer")
	}
}

func TestIsStderrTerminal_buffer(t *testing.T) {
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if o.IsStderrTerminal() {
		t.Error("IsStderrTerminal() should return false for *bytes.Buffer")
	}
}

func TestPrintVerbosePre(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &buf}
	o.PrintVerbosePre("openai/gpt-4o", "You are helpful.", "What is Go?")
	out := buf.String()
	if !strings.Contains(out, "openai/gpt-4o") {
		t.Errorf("PrintVerbosePre missing model: %q", out)
	}
	if !strings.Contains(out, "tokens_in=") {
		t.Errorf("PrintVerbosePre missing token count: %q", out)
	}
}

func TestPrintVerbosePost(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &buf}
	o.PrintVerbosePost(100, 50, 1.23)
	out := buf.String()
	if !strings.Contains(out, "in=100") {
		t.Errorf("PrintVerbosePost missing in tokens: %q", out)
	}
	if !strings.Contains(out, "out=50") {
		t.Errorf("PrintVerbosePost missing out tokens: %q", out)
	}
	if !strings.Contains(out, "time=1.23s") {
		t.Errorf("PrintVerbosePost missing time: %q", out)
	}
}

func TestSpinnerWriter_stopsSpinnerOnFirstWrite(t *testing.T) {
	var errBuf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	s := o.NewSpinner("working")

	var outBuf bytes.Buffer
	sw := NewSpinnerWriter(&outBuf, s)

	// Writing to SpinnerWriter should stop the spinner.
	n, err := sw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("SpinnerWriter.Write() error: %v", err)
	}
	if n != 5 {
		t.Errorf("SpinnerWriter.Write() = %d bytes, want 5", n)
	}
	if outBuf.String() != "hello" {
		t.Errorf("SpinnerWriter forwarded %q, want %q", outBuf.String(), "hello")
	}
	// Second write must not panic (spinner already stopped).
	_, _ = sw.Write([]byte(" world"))
}

func TestPrintDryRunHistory(t *testing.T) {
	var errBuf bytes.Buffer
	o := &Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	o.PrintDryRunHistory([]DryRunMessage{
		{Role: "user", Content: "what is k8s?"},
		{Role: "assistant", Content: "an orchestrator"},
	})
	got := errBuf.String()
	if !strings.Contains(got, "history user: what is k8s?") {
		t.Errorf("missing user line: %q", got)
	}
	if !strings.Contains(got, "history assistant: an orchestrator") {
		t.Errorf("missing assistant line: %q", got)
	}
}
