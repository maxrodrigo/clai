package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

func TestName(t *testing.T) {
	p := &Provider{name: "openai"}
	if got := p.Name(); got != "openai" {
		t.Errorf("Name() = %q, want %q", got, "openai")
	}
}

func TestName_ollama(t *testing.T) {
	p := &Provider{name: "ollama"}
	if got := p.Name(); got != "ollama" {
		t.Errorf("Name() = %q, want %q", got, "ollama")
	}
}

func TestBuildParams_basic(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model:  "gpt-4",
		System: "Be helpful",
		User:   "Hello",
	}

	params := p.buildParams(req)

	if params.Model != req.Model {
		t.Errorf("Model = %q, want %q", params.Model, req.Model)
	}
	// Should have 2 messages: system + user
	if len(params.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(params.Messages))
	}
}

func TestBuildParams_noSystem(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model: "gpt-4",
		User:  "Hello",
	}

	params := p.buildParams(req)

	// Should have 1 message: user only
	if len(params.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(params.Messages))
	}
}

func TestBuildParams_maxTokens(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model:     "gpt-4",
		User:      "Hello",
		MaxTokens: 500,
	}

	params := p.buildParams(req)

	if params.MaxTokens.Value != 500 {
		t.Errorf("MaxTokens = %d, want 500", params.MaxTokens.Value)
	}
}

func TestBuildParams_noMaxTokens(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model: "gpt-4",
		User:  "Hello",
	}

	params := p.buildParams(req)

	// MaxTokens should not be set (zero value)
	if params.MaxTokens.Valid() {
		t.Errorf("MaxTokens should not be set when not provided, got %d", params.MaxTokens.Value)
	}
}

func TestBuildParams_temperature(t *testing.T) {
	p := &Provider{name: "openai"}
	temp := 0.7
	req := provider.Request{
		Model:       "gpt-4",
		User:        "Hello",
		Temperature: &temp,
	}

	params := p.buildParams(req)

	if !params.Temperature.Valid() || params.Temperature.Value != temp {
		t.Errorf("Temperature = %v, want %v", params.Temperature.Value, temp)
	}
}

func TestBuildParams_jsonMode(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model:    "gpt-4",
		User:     "Return JSON",
		JSONMode: true,
	}

	params := p.buildParams(req)

	if params.ResponseFormat.OfJSONObject == nil {
		t.Error("ResponseFormat.OfJSONObject should be set when JSONMode is true")
	}
}

func TestBuildParams_noJSONMode(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model:    "gpt-4",
		User:     "Hello",
		JSONMode: false,
	}

	params := p.buildParams(req)

	if params.ResponseFormat.OfJSONObject != nil {
		t.Error("ResponseFormat.OfJSONObject should not be set when JSONMode is false")
	}
}

func TestBuildParams_messages(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: "system", Content: "Be concise"},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "A programming language."},
			{Role: "user", Content: "Who created it?"},
		},
	}

	params := p.buildParams(req)

	// Should have 4 messages: system + 3 turns
	if len(params.Messages) != 4 {
		t.Fatalf("len(Messages) = %d, want 4", len(params.Messages))
	}
	// First message should be system
	if params.Messages[0].OfSystem == nil {
		t.Errorf("Messages[0] should be system, got %+v", params.Messages[0])
	}
	// Second should be user
	if params.Messages[1].OfUser == nil {
		t.Errorf("Messages[1] should be user, got %+v", params.Messages[1])
	}
	// Third should be assistant
	if params.Messages[2].OfAssistant == nil {
		t.Errorf("Messages[2] should be assistant, got %+v", params.Messages[2])
	}
	// Fourth should be user
	if params.Messages[3].OfUser == nil {
		t.Errorf("Messages[3] should be user, got %+v", params.Messages[3])
	}
}

func TestBuildParams_singleShotUnchanged(t *testing.T) {
	p := &Provider{name: "openai"}
	req := provider.Request{
		Model:  "gpt-4",
		System: "Be helpful",
		User:   "Hello",
	}

	params := p.buildParams(req)

	// Should still produce 2 messages: system + user
	if len(params.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(params.Messages))
	}
	if params.Messages[0].OfSystem == nil {
		t.Errorf("Messages[0] should be system")
	}
	if params.Messages[1].OfUser == nil {
		t.Errorf("Messages[1] should be user")
	}
}

func TestApiError_nonAPIError(t *testing.T) {
	orig := errors.New("connection reset")
	got := apiError(orig)
	if !errors.Is(got, orig) {
		t.Errorf("apiError() should return the original error unchanged, got %v", got)
	}
}

func TestApiError_sdkError(t *testing.T) {
	// Create a test server that returns a 400 with an OpenAI error body.
	body := `{"error":{"message":"invalid model: fake-model","type":"invalid_request_error","code":"model_not_found"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	// Make a real SDK call that will fail with a 400.
	p := newProvider("openai", config.ProviderConfig{BaseURL: srv.URL, APIKey: "test-key"})

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
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if apiErr.Message != "invalid model: fake-model" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "invalid model: fake-model")
	}
}
