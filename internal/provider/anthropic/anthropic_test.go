package anthropic

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

func TestName(t *testing.T) {
	p := &Provider{}
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestApiError_nonAPIError(t *testing.T) {
	orig := errors.New("connection reset")
	got := apiError(orig)
	if got != orig {
		t.Errorf("apiError() should return the original error unchanged, got %v", got)
	}
}

func TestApiError_sdkError(t *testing.T) {
	// Create a test server that returns a 400 with a JSON error body.
	body := `{"type":"error","error":{"type":"invalid_request_error","message":"model not available"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	// Make a real SDK call that will fail with a 400.
	p := newProvider(config.ProviderConfig{BaseURL: srv.URL, APIKey: "test-key"})

	_, err := p.Complete(t.Context(), provider.Request{
		Model: "fake-model",
		User:  "hello",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The error should contain our API message.
	var opErr *provider.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *provider.OpError, got %T", err)
	}
	var apiErr *provider.APIError
	if !errors.As(opErr.Err, &apiErr) {
		t.Fatalf("expected *provider.APIError, got %T: %v", opErr.Err, opErr.Err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Message != "model not available" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "model not available")
	}
}

func TestExtractMessage(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "valid anthropic envelope",
			raw:  `{"type":"error","error":{"type":"invalid_request_error","message":"model not found"}}`,
			want: "model not found",
		},
		{
			name: "empty message",
			raw:  `{"type":"error","error":{"type":"invalid_request_error","message":""}}`,
			want: "",
		},
		{
			name: "invalid json",
			raw:  "not json",
			want: "",
		},
		{
			name: "missing error field",
			raw:  `{"type":"error"}`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMessage(tt.raw)
			if got != tt.want {
				t.Errorf("extractMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildParams_basic(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model:  "claude-3-haiku-20240307",
		System: "Be helpful",
		User:   "Hello",
	}

	params := p.buildParams(req)

	if params.Model != req.Model {
		t.Errorf("Model = %q, want %q", params.Model, req.Model)
	}
	if params.MaxTokens != defaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", params.MaxTokens, defaultMaxTokens)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(params.Messages))
	}
	if len(params.System) != 1 || params.System[0].Text != "Be helpful" {
		t.Errorf("System = %+v, want [{Text: Be helpful}]", params.System)
	}
}

func TestBuildParams_maxTokensOverride(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model:     "claude-3-haiku-20240307",
		User:      "Hello",
		MaxTokens: 500,
	}

	params := p.buildParams(req)

	if params.MaxTokens != 500 {
		t.Errorf("MaxTokens = %d, want 500", params.MaxTokens)
	}
}

func TestBuildParams_temperature(t *testing.T) {
	p := &Provider{}
	temp := 0.7
	req := provider.Request{
		Model:       "claude-3-haiku-20240307",
		User:        "Hello",
		Temperature: &temp,
	}

	params := p.buildParams(req)

	if !params.Temperature.Valid() || params.Temperature.Value != temp {
		t.Errorf("Temperature = %v, want %v", params.Temperature.Value, temp)
	}
}

func TestBuildParams_noSystem(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model: "claude-3-haiku-20240307",
		User:  "Hello",
	}

	params := p.buildParams(req)

	if len(params.System) != 0 {
		t.Errorf("System should be empty when not provided, got %+v", params.System)
	}
}

func TestBuildParams_thinkEnabled(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model: "claude-3-haiku-20240307",
		User:  "Think about this",
		Think: true,
	}

	params := p.buildParams(req)

	if params.Thinking.OfEnabled == nil {
		t.Fatal("Thinking.OfEnabled is nil, expected enabled config")
	}
	if params.Thinking.OfEnabled.BudgetTokens != int64(defaultThinkBudget) {
		t.Errorf("BudgetTokens = %d, want %d", params.Thinking.OfEnabled.BudgetTokens, defaultThinkBudget)
	}
}

func TestBuildParams_thinkBudgetOverride(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model:       "claude-3-haiku-20240307",
		User:        "Think about this",
		Think:       true,
		ThinkBudget: 5000,
	}

	params := p.buildParams(req)

	if params.Thinking.OfEnabled == nil {
		t.Fatal("Thinking.OfEnabled is nil")
	}
	if params.Thinking.OfEnabled.BudgetTokens != 5000 {
		t.Errorf("BudgetTokens = %d, want 5000", params.Thinking.OfEnabled.BudgetTokens)
	}
}

func TestBuildParams_noThinkNoThinking(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model: "claude-3-haiku-20240307",
		User:  "Hello",
		Think: false,
	}

	params := p.buildParams(req)

	if params.Thinking.OfEnabled != nil {
		t.Error("Thinking should not be set when Think is false")
	}
}

func TestBuildParams_messages(t *testing.T) {
	p := &Provider{}
	req := provider.Request{
		Model: "claude-3-haiku-20240307",
		Messages: []provider.Message{
			{Role: "system", Content: "Be concise"},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "A language."},
			{Role: "user", Content: "Who made it?"},
		},
	}

	params := p.buildParams(req)

	// System should be lifted into the dedicated system slot
	if len(params.System) != 1 || params.System[0].Text != "Be concise" {
		t.Errorf("System = %+v, want [{Text: Be concise}]", params.System)
	}
	// Should have 3 turn messages (system is not in Messages)
	if len(params.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(params.Messages))
	}
	// Verify roles: user, assistant, user
	if params.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role = %q, want user", params.Messages[0].Role)
	}
	if params.Messages[1].Role != "assistant" {
		t.Errorf("Messages[1].Role = %q, want assistant", params.Messages[1].Role)
	}
	if params.Messages[2].Role != "user" {
		t.Errorf("Messages[2].Role = %q, want user", params.Messages[2].Role)
	}
}
