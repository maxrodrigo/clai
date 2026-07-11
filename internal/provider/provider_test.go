package provider_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

// stub is a minimal Provider used in tests.
type stub struct{ name string }

func (s *stub) Name() string { return s.name }

func (s *stub) Complete(_ context.Context, _ provider.Request) (provider.Response, error) {
	return provider.Response{}, nil
}

func (s *stub) CompleteStream(_ context.Context, _ provider.Request, _ io.Writer) (provider.Response, error) {
	return provider.Response{}, nil
}

func (s *stub) Models(_ context.Context) ([]string, error) { return nil, nil }

func cfgWithProvider(name, baseURL string) *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			name: {BaseURL: baseURL, APIKey: "test"},
		},
	}
}

func TestGetByName_unknownFactory_withBaseURL_fallsBackToOpenAI(t *testing.T) {
	provider.ResetRegistry()
	provider.Register("openai", func(pc config.ProviderConfig) provider.Provider {
		return &stub{name: "openai"}
	})

	cfg := cfgWithProvider("localai", "http://localhost:8080/v1")
	prov, err := provider.GetByName("localai", cfg)
	if err != nil {
		t.Fatalf("expected OpenAI-compat fallback for provider with base_url, got error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestGetByName_unknownFactory_withoutBaseURL_errors(t *testing.T) {
	provider.ResetRegistry()
	provider.Register("openai", func(pc config.ProviderConfig) provider.Provider {
		return &stub{name: "openai"}
	})

	cfg := cfgWithProvider("opanai", "") // typo, no base_url
	_, err := provider.GetByName("opanai", cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider without base_url, got nil")
	}
	if !strings.Contains(err.Error(), "opanai") {
		t.Errorf("error message should mention the unknown provider name, got: %v", err)
	}
}

func TestGetByName_unknownProvider_notInConfig_errors(t *testing.T) {
	provider.ResetRegistry()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{},
	}
	_, err := provider.GetByName("gogle", cfg)
	if err == nil {
		t.Fatal("expected error for provider not in config, got nil")
	}
	if !strings.Contains(err.Error(), "gogle") {
		t.Errorf("error message should mention the unknown provider name, got: %v", err)
	}
}

func TestRegister_duplicate_panics(t *testing.T) {
	provider.ResetRegistry()
	provider.Register("test", func(pc config.ProviderConfig) provider.Provider {
		return &stub{name: "test"}
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	provider.Register("test", func(pc config.ProviderConfig) provider.Provider {
		return &stub{name: "test"}
	})
}

func TestGet_validModel(t *testing.T) {
	provider.ResetRegistry()
	provider.Register("openai", func(pc config.ProviderConfig) provider.Provider {
		return &stub{name: "openai"}
	})

	cfg := &config.Config{
		Model: "openai/gpt-4o",
		Providers: map[string]config.ProviderConfig{
			"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
		},
	}
	prov, err := provider.Get(cfg.Model, cfg)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if prov == nil {
		t.Fatal("Get() = nil, want non-nil provider")
	}
}

func TestGet_noProviderPrefix(t *testing.T) {
	provider.ResetRegistry()

	cfg := &config.Config{
		Model:     "gpt-4o",
		Providers: map[string]config.ProviderConfig{},
	}
	_, err := provider.Get(cfg.Model, cfg)
	if err == nil {
		t.Fatal("Get() without provider prefix should error")
	}
}
