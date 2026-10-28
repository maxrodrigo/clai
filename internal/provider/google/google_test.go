package google

import (
	"testing"

	"github.com/maxrodrigo/clai/internal/provider"
)

func TestBuildConfig_Defaults(t *testing.T) {
	p := &Provider{name: "gemini"}
	req := provider.Request{Model: "gemini-2.5-flash", User: "hello"}
	cfg := p.buildConfig(req)

	if cfg.SystemInstruction != nil {
		t.Error("expected nil SystemInstruction for empty system")
	}
	if cfg.Temperature != nil {
		t.Error("expected nil Temperature when not set")
	}
	if cfg.MaxOutputTokens != 0 {
		t.Error("expected zero MaxOutputTokens when not set")
	}
	if cfg.ResponseMIMEType != "" {
		t.Error("expected empty ResponseMIMEType when JSONMode is false")
	}
	if cfg.ThinkingConfig != nil {
		t.Error("expected nil ThinkingConfig when Think is false")
	}
}

func TestBuildConfig_AllOptions(t *testing.T) {
	p := &Provider{name: "gemini"}
	temp := 0.7
	req := provider.Request{
		Model:       "gemini-2.5-flash",
		User:        "hello",
		System:      "You are a helper.",
		Temperature: &temp,
		MaxTokens:   1024,
		JSONMode:    true,
		Think:       true,
		ThinkBudget: 5000,
	}
	cfg := p.buildConfig(req)

	if cfg.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction to be set")
	}
	if cfg.Temperature == nil || *cfg.Temperature != float32(0.7) {
		t.Errorf("expected Temperature 0.7, got %v", cfg.Temperature)
	}
	if cfg.MaxOutputTokens != 1024 {
		t.Errorf("expected MaxOutputTokens 1024, got %v", cfg.MaxOutputTokens)
	}
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("expected application/json, got %q", cfg.ResponseMIMEType)
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 5000 {
		t.Errorf("expected ThinkingBudget 5000, got %v", cfg.ThinkingConfig)
	}
}

func TestBuildConfig_ThinkDefaultBudget(t *testing.T) {
	p := &Provider{name: "vertex"}
	req := provider.Request{
		Model: "gemini-2.5-pro",
		User:  "hello",
		Think: true,
	}
	cfg := p.buildConfig(req)

	if cfg.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig to be set")
	}
	if cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != int32(defaultThinkBudget) {
		t.Errorf("expected default budget %d, got %v", defaultThinkBudget, cfg.ThinkingConfig.ThinkingBudget)
	}
}

func TestBuildContents_messages(t *testing.T) {
	req := provider.Request{
		Model: "gemini-2.5-flash",
		Messages: []provider.Message{
			{Role: "system", Content: "Be concise"},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "A language."},
			{Role: "user", Content: "Who made it?"},
		},
	}

	contents := buildContents(req)

	// Should have 3 content items (system is lifted out by Turns)
	if len(contents) != 3 {
		t.Fatalf("len(contents) = %d, want 3", len(contents))
	}
	// First should be user role
	if contents[0].Role != "user" {
		t.Errorf("contents[0].Role = %q, want user", contents[0].Role)
	}
	// Second should be model role (Google uses "model" for assistant)
	if contents[1].Role != "model" {
		t.Errorf("contents[1].Role = %q, want model", contents[1].Role)
	}
	// Third should be user role
	if contents[2].Role != "user" {
		t.Errorf("contents[2].Role = %q, want user", contents[2].Role)
	}
	// Verify content text
	if contents[0].Parts[0].Text != "What is Go?" {
		t.Errorf("contents[0] text = %q, want 'What is Go?'", contents[0].Parts[0].Text)
	}
	if contents[1].Parts[0].Text != "A language." {
		t.Errorf("contents[1] text = %q, want 'A language.'", contents[1].Parts[0].Text)
	}
	if contents[2].Parts[0].Text != "Who made it?" {
		t.Errorf("contents[2] text = %q, want 'Who made it?'", contents[2].Parts[0].Text)
	}
}

func TestBuildConfig_systemFromMessages(t *testing.T) {
	p := &Provider{name: "gemini"}
	req := provider.Request{
		Model: "gemini-2.5-flash",
		Messages: []provider.Message{
			{Role: "system", Content: "You are a helper."},
			{Role: "user", Content: "hello"},
		},
	}

	cfg := p.buildConfig(req)

	if cfg.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction to be set")
	}
	if len(cfg.SystemInstruction.Parts) != 1 || cfg.SystemInstruction.Parts[0].Text != "You are a helper." {
		t.Errorf("SystemInstruction = %+v, want text 'You are a helper.'", cfg.SystemInstruction)
	}
}
