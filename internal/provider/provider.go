// Package provider defines the interface for AI model backends.
package provider

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/maxrodrigo/clai/internal/config"
)

// Provider defines the contract for an AI model backend.
type Provider interface {
	// Name returns the provider identifier (e.g. "openai", "anthropic").
	Name() string
	// Complete sends a request and returns the full response.
	Complete(ctx context.Context, req Request) (Response, error)
	// CompleteStream sends a request and streams tokens to w as they arrive.
	// The final accumulated response is also returned.
	CompleteStream(ctx context.Context, req Request, w io.Writer) (Response, error)
	// Models returns a list of available model IDs.
	Models(ctx context.Context) ([]string, error)
}

// Request represents the input to a model call.
type Request struct {
	Model       string
	System      string
	User        string
	Temperature *float64 // nil means use provider default
	MaxTokens   int
	Think       bool
	ThinkBudget int
	JSONMode    bool
}

// Response represents the output from a model call.
type Response struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// factory is a function that creates a Provider from a ProviderConfig.
type factory func(pc config.ProviderConfig) Provider

var registry = map[string]factory{}

// Register adds a provider factory to the registry.
// Panics if a factory is already registered for the given name (programming error).
func Register(name string, factory factory) {
	if _, exists := registry[name]; exists {
		panic("provider: duplicate registration for " + name)
	}
	registry[name] = factory
}

// resetRegistry clears all registered providers (testing only).
func resetRegistry() {
	registry = map[string]factory{}
}

// Get returns a Provider for the given model string (e.g. "openai/gpt-4o").
func Get(model string, cfg *config.Config) (Provider, error) {
	providerName := cfg.ModelProvider()
	if providerName == "" {
		return nil, fmt.Errorf("model %q must be in provider/model format (e.g. openai/gpt-4o)", model)
	}
	return GetByName(providerName, cfg)
}

// GetByName returns a Provider for a provider name without needing a
// full model string. Used by listModels and similar callers that operate on
// providers directly.
func GetByName(name string, cfg *config.Config) (Provider, error) {
	pc, err := cfg.GetProvider(name)
	if err != nil {
		return nil, err
	}

	factory, ok := registry[name]
	if !ok {
		// Fall back to OpenAI-compatible only when the provider has an explicit
		// base_url configured — the user opted into OpenAI-compat for this provider.
		// Without a base_url there is nothing meaningful to connect to, so we
		// error rather than silently misdirecting the request.
		if pc.BaseURL == "" {
			registered := registeredNames()
			return nil, fmt.Errorf(
				"unsupported provider %q — no factory registered and no base_url configured\n\nKnown providers: %s",
				name, strings.Join(registered, ", "),
			)
		}
		factory, ok = registry["openai"]
		if !ok {
			return nil, fmt.Errorf("internal: openai factory not registered")
		}
	}

	// Pre-flight: providers without a base_url (remote APIs) require an API key.
	// Providers with a base_url may be local (e.g., Ollama) and don't need one.
	if pc.APIKey == "" && pc.BaseURL == "" && requiresAPIKey(name) {
		return nil, fmt.Errorf("no API key configured for %s (set %s)", name, envKeyHint(name))
	}

	return factory(pc), nil
}

// requiresAPIKey returns true for providers known to need authentication.
func requiresAPIKey(name string) bool {
	switch name {
	case "ollama":
		return false
	default:
		return true
	}
}

// envKeyHint returns the conventional env var name for a provider's API key.
func envKeyHint(name string) string {
	switch name {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "bedrock":
		return "AWS_BEARER_TOKEN_BEDROCK"
	default:
		return strings.ToUpper(name) + "_API_KEY"
	}
}

// registeredNames returns a sorted list of registered provider names.
func registeredNames() []string {
	return slices.Sorted(maps.Keys(registry))
}
