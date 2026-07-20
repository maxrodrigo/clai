package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxrodrigo/clai/internal/conversation"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/provider"
	"github.com/maxrodrigo/clai/internal/source"
)

func TestBuildMessages_InlineWithoutInput(t *testing.T) {
	p := &prompt.Prompt{Content: "Explain this"}
	opts := PromptOptions{InlinePrompt: "Explain this"}

	sys, user, err := buildMessages(p, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sys != "" {
		t.Errorf("expected empty system prompt, got %q", sys)
	}
	if user != "Explain this" {
		t.Errorf("expected user message to be prompt content, got %q", user)
	}
}

func TestBuildMessages_NoInputNoInline(t *testing.T) {
	p := &prompt.Prompt{Content: "Summarize"}
	opts := PromptOptions{}

	_, _, err := buildMessages(p, opts, nil)
	if err == nil {
		t.Fatal("expected error for no input and no inline prompt")
	}
}

func TestBuildMessages_WithInput(t *testing.T) {
	p := &prompt.Prompt{Content: "You are a summarizer."}
	opts := PromptOptions{}
	input := []byte("Some article text here.")

	sys, user, err := buildMessages(p, opts, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sys != "You are a summarizer." {
		t.Errorf("expected system prompt to be prompt content, got %q", sys)
	}
	if user != "Some article text here." {
		t.Errorf("expected user message to be input, got %q", user)
	}
}

func TestPromptEnvKey(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"my-prompt", "MY_PROMPT"},
		{"alice/review", "ALICE_REVIEW"},
		{"code-review", "CODE_REVIEW"},
		{"simple", "SIMPLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := promptEnvKey(tt.name)
			if got != tt.want {
				t.Errorf("promptEnvKey(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestResolvePrompt_Inline(t *testing.T) {
	opts := PromptOptions{InlinePrompt: "Hello world"}
	p, err := resolvePrompt(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Content != "Hello world" {
		t.Errorf("expected content %q, got %q", "Hello world", p.Content)
	}
	if p.Path != prompt.LiteralPath {
		t.Errorf("expected path %q, got %q", prompt.LiteralPath, p.Path)
	}
}

func TestResolvePrompt_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte("---\ndescription: test\n---\nDo the thing."), 0644); err != nil {
		t.Fatal(err)
	}

	opts := PromptOptions{PromptFile: path}
	p, err := resolvePrompt(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Content != "Do the thing." {
		t.Errorf("expected content %q, got %q", "Do the thing.", p.Content)
	}
}

func TestResolvePrompt_FileMissing(t *testing.T) {
	opts := PromptOptions{PromptFile: "/nonexistent/path.md"}
	_, err := resolvePrompt(opts)
	if err == nil {
		t.Fatal("expected error for missing prompt file")
	}
}

func TestExplicitPrompt(t *testing.T) {
	tests := []struct {
		name string
		opts PromptOptions
		want bool
	}{
		{"named prompt", PromptOptions{PromptName: "x"}, true},
		{"inline prompt", PromptOptions{InlinePrompt: "x"}, true},
		{"prompt file", PromptOptions{PromptFile: "x"}, true},
		{"conversation only", PromptOptions{Conversation: "some-chat"}, false},
		{"zero opts", PromptOptions{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := explicitPrompt(tt.opts); got != tt.want {
				t.Errorf("explicitPrompt(%+v) = %v, want %v", tt.opts, got, tt.want)
			}
		})
	}
}

func TestPersistTurnStoresEmptySystemLineWithModel(t *testing.T) {
	t.Setenv("CLAI_CONVERSATIONS_DIR", t.TempDir())

	conv, err := conversation.Open("t")
	if err != nil {
		t.Fatal(err)
	}

	var outBuf, errBuf bytes.Buffer
	rt := &Runtime{
		Output: &output.Output{Stdout: &outBuf, Stderr: &errBuf},
		Input:  &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}},
	}

	persistTurn(rt, conv, true, "", "openai/gpt-4.1", "hi", provider.Response{Content: "yo"})

	msgs, skipped, err := conv.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("expected no skipped lines, got %d", skipped)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected first message role 'system', got %q", msgs[0].Role)
	}
	if msgs[0].Content != "" {
		t.Errorf("expected empty system content, got %q", msgs[0].Content)
	}
	if msgs[0].Model != "openai/gpt-4.1" {
		t.Errorf("expected system model 'openai/gpt-4.1', got %q", msgs[0].Model)
	}
}

func TestPromptConversationDryRunShowsHistory(t *testing.T) {
	// Seed a conversation with history.
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	c, err := conversation.Open("test-conv")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now()
	_ = c.Append(conversation.Message{Role: "system", Content: "be brief", Model: "openai/gpt-4.1", TS: ts})
	_ = c.Append(conversation.Message{Role: "user", Content: "what is k8s?", TS: ts})
	_ = c.Append(conversation.Message{Role: "assistant", Content: "an orchestrator", Model: "openai/gpt-4.1", TS: ts, TokensIn: 25, TokensOut: 150})

	var outBuf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &outBuf, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}
	rt := &Runtime{Output: out, Input: in}

	opts := PromptOptions{
		InlinePrompt: "explain more",
		DryRun:       true,
		Conversation: "test-conv",
	}

	err = Prompt(context.Background(), rt, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := errBuf.String()
	if !strings.Contains(got, "history user: what is k8s?") {
		t.Errorf("missing history user line in stderr: %q", got)
	}
	if !strings.Contains(got, "history assistant: an orchestrator") {
		t.Errorf("missing history assistant line in stderr: %q", got)
	}
	// Should inherit model from conversation history
	if !strings.Contains(got, "openai/gpt-4.1") {
		t.Errorf("should inherit model from conversation, stderr: %q", got)
	}
}

func TestPromptConversationInlineWithoutInputInheritsSystem(t *testing.T) {
	// With no piped input, -e text becomes the USER message (buildMessages),
	// so it is not a system-prompt override: the conversation's stored
	// system prompt must be inherited, not dropped.
	t.Setenv("CLAI_CONVERSATIONS_DIR", t.TempDir())

	c, err := conversation.Open("persona")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now()
	_ = c.Append(conversation.Message{Role: "system", Content: "answer like a pirate", Model: "openai/gpt-4.1", TS: ts})
	_ = c.Append(conversation.Message{Role: "user", Content: "explain k8s", TS: ts})
	_ = c.Append(conversation.Message{Role: "assistant", Content: "arr, containers", TS: ts})

	var outBuf, errBuf bytes.Buffer
	rt := &Runtime{
		Output: &output.Output{Stdout: &outBuf, Stderr: &errBuf},
		Input:  &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}},
	}

	err = Prompt(context.Background(), rt, PromptOptions{
		InlinePrompt: "and swarm?", // no stdin: this is the user message
		DryRun:       true,
		Conversation: "persona",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := errBuf.String()
	if !strings.Contains(got, "system: answer like a pirate") {
		t.Errorf("stored system prompt not inherited, stderr: %q", got)
	}
	if !strings.Contains(got, "user: and swarm?") {
		t.Errorf("-e text should be the user message, stderr: %q", got)
	}
}

func TestPromptConversationLatestNoneErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var outBuf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &outBuf, Stderr: &errBuf}
	in := &source.Input{Stdin: strings.NewReader(""), Stderr: &bytes.Buffer{}}
	rt := &Runtime{Output: out, Input: in}

	opts := PromptOptions{
		InlinePrompt: "hello",
		Conversation: "-",
	}

	err := Prompt(context.Background(), rt, opts)
	if err == nil {
		t.Fatal("expected error for -c - with empty dir")
	}
	if !strings.Contains(err.Error(), "no conversations found") {
		t.Errorf("expected 'no conversations found' error, got: %v", err)
	}
}

func TestPromptConversationBinaryInputRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	var outBuf, errBuf bytes.Buffer
	out := &output.Output{Stdout: &outBuf, Stderr: &errBuf}
	// Input with null bytes
	in := &source.Input{Stdin: strings.NewReader("hello\x00world"), Stderr: &bytes.Buffer{}}
	rt := &Runtime{Output: out, Input: in}

	opts := PromptOptions{
		InlinePrompt: "summarize this",
		Conversation: "some-conv",
	}

	err := Prompt(context.Background(), rt, opts)
	if err == nil {
		t.Fatal("expected error for binary input with -c")
	}
	if !strings.Contains(err.Error(), "binary input") {
		t.Errorf("expected 'binary input' error, got: %v", err)
	}
}
