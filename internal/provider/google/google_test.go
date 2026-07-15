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
