package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/conversation"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/source"
	"github.com/maxrodrigo/clai/internal/strategy"
	"github.com/maxrodrigo/clai/internal/testsupport"

	// Register providers for tests.
	_ "github.com/maxrodrigo/clai/internal/provider/anthropic"
	_ "github.com/maxrodrigo/clai/internal/provider/bedrock"
	_ "github.com/maxrodrigo/clai/internal/provider/openai"
)

// TestMain isolates the environment from the developer's real configuration,
// then registers prompt and strategy sources against the in-repo data
// directory and the (now empty) user config directory. Tests therefore see
// only system prompts and strategies, regardless of the host machine.
func TestMain(m *testing.M) {
	cleanup := testsupport.IsolateConfig()

	dataDir := testsupport.DataDir()
	warn := output.WarnWriter(os.Stderr)
	prompt.RegisterDefaultSources(dataDir, config.Dir(), warn)
	strategy.Init(dataDir, config.Dir(), warn)

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestCLINoArgs(t *testing.T) {
	var outBuf bytes.Buffer
	out := &output.Output{Stdout: &outBuf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// No args should show help (not error) — modern CLI pattern.
	if err != nil {
		t.Errorf("no args should show help without error, got: %v", err)
	}
	if !strings.Contains(outBuf.String(), "Usage:") {
		t.Errorf("expected help output, got: %q", outBuf.String())
	}
}

func TestCLIStrategies(t *testing.T) {
	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"strategy", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("strategy list: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "cot") {
		t.Errorf("output missing 'cot': %q", got)
	}
	if !strings.Contains(got, "self-refine") {
		t.Errorf("output missing 'self-refine': %q", got)
	}
}

// TestCLIStrategySentinel guards against a regression where "none" (and "")
// resolve to a nil *Strategy, which the show/path subcommands then dereferenced.
func TestCLIStrategySentinel(t *testing.T) {
	for _, sub := range []string{"show", "path"} {
		t.Run(sub, func(t *testing.T) {
			out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
			in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

			cmd := NewRoot(out, in)
			cmd.SetArgs([]string{"strategy", sub, "none"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("strategy %s none: expected error, got nil", sub)
			}
			if !strings.Contains(err.Error(), "not a strategy") {
				t.Errorf("strategy %s none: expected 'not a strategy' error, got: %v", sub, err)
			}
		})
	}
}

func TestCLIPrompts(t *testing.T) {
	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"prompt", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt list: %v", err)
	}
	if !strings.Contains(buf.String(), "summarize") {
		t.Errorf("output missing 'summarize': %q", buf.String())
	}
}

func TestCLIDryRun(t *testing.T) {
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var buf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader("test input"), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--dry-run", "summarize"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// Dry-run prints to stderr
	got := errBuf.String()
	if !strings.Contains(got, "[clai] dry-run:") {
		t.Errorf("output missing '[clai] dry-run:': %q", got)
	}
	if !strings.Contains(got, "[clai] user:") {
		t.Errorf("output missing '[clai] user:': %q", got)
	}
	if !strings.Contains(got, "~tokens_in=") {
		t.Errorf("output missing token estimate: %q", got)
	}
}

func TestCLIUnknownPrompt(t *testing.T) {
	var errBuf bytes.Buffer
	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader("test"), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"nonexistent-prompt-xyz"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unknown prompt, got nil")
	}
}

func TestCLINamedPromptNoInput(t *testing.T) {
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var errBuf bytes.Buffer
	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	// Simulate terminal (no piped input) - IsStdinTerminal returns false for strings.Reader
	// but readStdin also calls IsStdinTerminal which returns false for non-*os.File.
	// To simulate "no input" we use an empty reader.
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"summarize"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for named prompt without input")
	}
	if !strings.Contains(err.Error(), "no input provided") {
		t.Errorf("expected 'no input provided' error, got: %v", err)
	}
}

func TestCLIInlinePromptNoInput(t *testing.T) {
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var buf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &errBuf}
	// Empty reader simulates no piped input.
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--dry-run", "-e", "tell me a joke"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("inline prompt without input should work: %v", err)
	}
	// Inline prompt becomes user message when no input
	got := errBuf.String()
	if !strings.Contains(got, "tell me a joke") {
		t.Errorf("expected inline prompt in output, got: %q", got)
	}
}

func TestCLIInlinePromptWithInput(t *testing.T) {
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var buf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader("some content to summarize"), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--dry-run", "-e", "summarize this"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("inline prompt with input: %v", err)
	}
	got := errBuf.String()
	// With input: -e becomes system prompt, input becomes user message
	if !strings.Contains(got, "system:") || !strings.Contains(got, "summarize this") {
		t.Errorf("expected system prompt with inline content, got: %q", got)
	}
	if !strings.Contains(got, "some content") {
		t.Errorf("expected input in user message, got: %q", got)
	}
}

func TestCLIInlineAndFile_mutuallyExclusive(t *testing.T) {
	var errBuf bytes.Buffer
	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"-e", "inline prompt", "-f", "some-file.md"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for -e and -f together, got nil")
	}
	if !strings.Contains(err.Error(), "cannot use both") {
		t.Errorf("expected 'cannot use both' error, got: %v", err)
	}
}

func TestCLIConversationInvalidName(t *testing.T) {
	var errBuf bytes.Buffer
	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader("test"), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"-c", "INVALID NAME!", "-e", "hello"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid conversation name")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("expected UsageError, got: %T: %v", err, err)
	}
}

func TestCLIConversationReservedTokensPassThrough(t *testing.T) {
	// Reserved tokens "-" and "+" should not trigger validation errors.
	// They'll fail downstream (e.g. no conversations found), but not at validation.
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	for _, token := range []string{"-", "+"} {
		t.Run(token, func(t *testing.T) {
			var errBuf bytes.Buffer
			out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &errBuf}
			in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

			cmd := NewRoot(out, in)
			cmd.SetErr(&errBuf)
			cmd.SetArgs([]string{"-c", token, "-e", "hello"})
			err := cmd.Execute()
			// Will error downstream (no conversations for "-", or no input for "+"),
			// but must NOT be a UsageError from validation.
			if err != nil {
				var ue *UsageError
				if errors.As(err, &ue) {
					t.Errorf("reserved token %q should not produce UsageError, got: %v", token, err)
				}
			}
		})
	}
}

func seedConversation(t *testing.T, name string) {
	t.Helper()
	c, err := conversation.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now()
	_ = c.Append(conversation.Message{Role: "system", Content: "be brief", Model: "openai/gpt-4.1", TS: ts})
	_ = c.Append(conversation.Message{Role: "user", Content: "what is k8s?", TS: ts})
	_ = c.Append(conversation.Message{Role: "assistant", Content: "an orchestrator", Model: "openai/gpt-4.1", TS: ts, TokensIn: 25, TokensOut: 150})
}

func TestCLIConversationList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	seedConversation(t, "my-chat")

	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"conversation", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conversation list: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "my-chat") {
		t.Errorf("output missing 'my-chat': %q", got)
	}
	if !strings.Contains(got, "NAME") {
		t.Errorf("output missing header: %q", got)
	}
}

func TestCLIConversationShow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	seedConversation(t, "show-test")

	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"conversation", "show", "show-test"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conversation show: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "what is k8s?") {
		t.Errorf("output missing user message: %q", got)
	}
	if !strings.Contains(got, "an orchestrator") {
		t.Errorf("output missing assistant message: %q", got)
	}
}

func TestCLIConversationRename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	seedConversation(t, "old-conv")

	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"conversation", "rename", "old-conv", "new-conv"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conversation rename: %v", err)
	}
	// Verify rename occurred.
	if _, err := os.Stat(filepath.Join(dir, "new-conv.jsonl")); err != nil {
		t.Errorf("new file not found after rename: %v", err)
	}
}

func TestCLIConversationRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	seedConversation(t, "doomed")

	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"conversation", "remove", "doomed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conversation remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "doomed.jsonl")); !os.IsNotExist(err) {
		t.Errorf("file should have been removed: %v", err)
	}
}

func TestCLIConversationRemoveOlderThan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	seedConversation(t, "ancient")
	seedConversation(t, "recent")

	// Make "ancient" look old.
	old := time.Now().Add(-40 * 24 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "ancient.jsonl"), old, old)

	var buf bytes.Buffer
	out := &output.Output{Stdout: &buf, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"conversation", "remove", "--older-than", "30d"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conversation remove --older-than: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ancient.jsonl")); !os.IsNotExist(err) {
		t.Errorf("ancient should have been removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "recent.jsonl")); err != nil {
		t.Errorf("recent should survive: %v", err)
	}
}

func TestCLIConversationShowMissingErrors(t *testing.T) {
	t.Setenv("CLAI_CONVERSATIONS_DIR", t.TempDir())
	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetArgs([]string{"conversation", "show", "does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Error("show on a missing conversation should error, consistent with remove/rename")
	}
}

func TestCLIConversationRemoveNameAndOlderThanConflict(t *testing.T) {
	t.Setenv("CLAI_CONVERSATIONS_DIR", t.TempDir())
	seedConversation(t, "keep-me")

	out := &output.Output{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}

	cmd := NewRoot(out, in)
	cmd.SetArgs([]string{"conversation", "remove", "keep-me", "--older-than", "30d"})
	err := cmd.Execute()
	if err == nil {
		t.Error("remove with both a name and --older-than should be a usage error")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("want UsageError, got %T: %v", err, err)
	}
}

func TestParseDurationRejectsNegative(t *testing.T) {
	// --older-than -30d would put the cutoff in the future and remove
	// every conversation.
	for _, s := range []string{"-30d", "-720h"} {
		if _, err := parseDuration(s); err == nil {
			t.Errorf("parseDuration(%q) should error", s)
		}
	}
	if d, err := parseDuration("30d"); err != nil || d != 30*24*time.Hour {
		t.Errorf("parseDuration(30d) = %v, %v", d, err)
	}
}
