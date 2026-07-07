package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maxrodrigo/clai/internal/prompt"
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
