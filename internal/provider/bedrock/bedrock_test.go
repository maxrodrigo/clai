package bedrock

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maxrodrigo/clai/internal/provider"
)

func TestBuildBody_thinkEnablesReasoningConfig(t *testing.T) {
	body := buildBody(provider.Request{User: "hi", Think: true})

	rc, ok := body.AdditionalModelRequestFields["reasoning_config"].(reasoningConfig)
	if !ok {
		t.Fatalf("reasoning_config missing or wrong type: %#v", body.AdditionalModelRequestFields)
	}
	if rc.Type != "enabled" {
		t.Errorf("reasoning_config.type = %q, want %q", rc.Type, "enabled")
	}
	if rc.BudgetTokens != defaultThinkBudget {
		t.Errorf("budget_tokens = %d, want %d", rc.BudgetTokens, defaultThinkBudget)
	}
	if body.InferenceConfig == nil || body.InferenceConfig.MaxTokens <= rc.BudgetTokens {
		t.Errorf("maxTokens must exceed the reasoning budget, got %+v", body.InferenceConfig)
	}
}

func TestBuildBody_thinkBudgetOverride(t *testing.T) {
	body := buildBody(provider.Request{User: "hi", Think: true, ThinkBudget: 2048})
	rc := body.AdditionalModelRequestFields["reasoning_config"].(reasoningConfig)
	if rc.BudgetTokens != 2048 {
		t.Errorf("budget_tokens = %d, want 2048", rc.BudgetTokens)
	}
}

func TestBuildBody_thinkDropsTemperature(t *testing.T) {
	temp := 0.7
	body := buildBody(provider.Request{User: "hi", Think: true, Temperature: &temp})
	if body.InferenceConfig != nil && body.InferenceConfig.Temperature != nil {
		t.Error("temperature must be omitted when thinking is enabled")
	}
}

func TestBuildBody_noThinkKeepsTemperature(t *testing.T) {
	temp := 0.5
	body := buildBody(provider.Request{User: "hi", Temperature: &temp})
	if body.AdditionalModelRequestFields != nil {
		t.Error("reasoning_config must be absent without --think")
	}
	if body.InferenceConfig == nil || body.InferenceConfig.Temperature == nil {
		t.Fatal("temperature should be set when provided and not thinking")
	}
	if *body.InferenceConfig.Temperature != temp {
		t.Errorf("temperature = %v, want %v", *body.InferenceConfig.Temperature, temp)
	}
}

// newTestProvider creates a Provider pointing at a test server.
func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Provider{
		baseURL: srv.URL,
		apiKey:  "test-key",
		client:  srv.Client(),
	}
}

func TestName(t *testing.T) {
	p := &Provider{}
	if got := p.Name(); got != "bedrock" {
		t.Errorf("Name() = %q, want %q", got, "bedrock")
	}
}

func TestComplete_success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/converse") {
			t.Errorf("path = %s, want to end with /converse", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}

		// Verify request body
		var reqBody converseRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Content[0].Text != "Hello" {
			t.Errorf("unexpected messages: %+v", reqBody.Messages)
		}

		resp := converseResponse{}
		resp.Output.Message.Content = []converseTextBlock{{Text: "Hi there!"}}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	p := newTestProvider(t, handler)
	req := provider.Request{
		Model: "anthropic.claude-v2",
		User:  "Hello",
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hi there!")
	}
	if resp.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.OutputTokens)
	}
}

func TestComplete_withSystem(t *testing.T) {
	var captured converseRequest

	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)

		resp := converseResponse{}
		resp.Output.Message.Content = []converseTextBlock{{Text: "ok"}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	p := newTestProvider(t, handler)
	req := provider.Request{
		Model:  "anthropic.claude-v2",
		System: "Be helpful",
		User:   "Hello",
	}

	_, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	if len(captured.System) != 1 || captured.System[0].Text != "Be helpful" {
		t.Errorf("System = %+v, want [{Text: Be helpful}]", captured.System)
	}
}

func TestComplete_noCredentials(t *testing.T) {
	p := &Provider{baseURL: "http://localhost", apiKey: "", client: http.DefaultClient}

	_, err := p.Complete(context.Background(), provider.Request{User: "hi"})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "missing credentials") {
		t.Errorf("error = %v, want to contain 'missing credentials'", err)
	}
}

func TestComplete_httpError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	p := newTestProvider(t, handler)
	_, err := p.Complete(context.Background(), provider.Request{Model: "m", User: "hi"})

	if err == nil {
		t.Fatal("expected error")
	}
	var opErr *provider.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected OpError, got %T", err)
	}
	if opErr.Provider != "bedrock" {
		t.Errorf("Provider = %q, want %q", opErr.Provider, "bedrock")
	}
}

func TestComplete_apiError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(converseErrorResponse{Message: "invalid model"})
	}

	p := newTestProvider(t, handler)
	_, err := p.Complete(context.Background(), provider.Request{Model: "bad", User: "hi"})

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("error = %v, want to contain 'invalid model'", err)
	}
}

// buildEventStreamFrame constructs a minimal AWS EventStream frame.
func buildEventStreamFrame(eventType string, payload []byte) []byte {
	// Header: ":event-type" with string value
	headerName := ":event-type"
	headerValue := eventType

	var headers bytes.Buffer
	headers.WriteByte(byte(len(headerName)))
	headers.WriteString(headerName)
	headers.WriteByte(7) // string type
	_ = binary.Write(&headers, binary.BigEndian, uint16(len(headerValue)))
	headers.WriteString(headerValue)

	headersBytes := headers.Bytes()
	headersLen := len(headersBytes)
	payloadLen := len(payload)

	// Total = 12 (prelude) + headers + payload + 4 (message CRC)
	totalLen := 12 + headersLen + payloadLen + 4

	var frame bytes.Buffer
	_ = binary.Write(&frame, binary.BigEndian, uint32(totalLen))
	_ = binary.Write(&frame, binary.BigEndian, uint32(headersLen))
	_ = binary.Write(&frame, binary.BigEndian, uint32(0)) // prelude CRC (ignored)
	frame.Write(headersBytes)
	frame.Write(payload)
	_ = binary.Write(&frame, binary.BigEndian, uint32(0)) // message CRC (ignored)

	return frame.Bytes()
}

func TestCompleteStream_success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/converse-stream") {
			t.Errorf("path = %s, want to end with /converse-stream", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")

		// Send delta events
		delta1 := `{"delta":{"text":"Hello "}}`
		w.Write(buildEventStreamFrame("contentBlockDelta", []byte(delta1)))

		delta2 := `{"delta":{"text":"world!"}}`
		w.Write(buildEventStreamFrame("contentBlockDelta", []byte(delta2)))

		// Send metadata with token counts
		metadata := `{"usage":{"inputTokens":10,"outputTokens":5}}`
		w.Write(buildEventStreamFrame("metadata", []byte(metadata)))
	}

	p := newTestProvider(t, handler)
	req := provider.Request{
		Model: "anthropic.claude-v2",
		User:  "Hello",
	}

	var buf bytes.Buffer
	resp, err := p.CompleteStream(context.Background(), req, &buf)
	if err != nil {
		t.Fatalf("CompleteStream() error: %v", err)
	}

	// Check streamed output
	if buf.String() != "Hello world!" {
		t.Errorf("streamed = %q, want %q", buf.String(), "Hello world!")
	}

	// Check accumulated response
	if resp.Content != "Hello world!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello world!")
	}
	if resp.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.OutputTokens)
	}
}

func TestCompleteStream_errorEvent(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")

		errPayload := `{"message":"rate limit exceeded"}`
		w.Write(buildEventStreamFrame("throttlingException", []byte(errPayload)))
	}

	p := newTestProvider(t, handler)
	_, err := p.CompleteStream(context.Background(), provider.Request{Model: "m", User: "hi"}, &bytes.Buffer{})

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "throttlingException") || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %v, want to contain throttlingException and rate limit", err)
	}
}

func TestParseEventType(t *testing.T) {
	tests := []struct {
		name    string
		headers []byte
		want    string
	}{
		{
			name:    "valid event type",
			headers: buildEventHeaders(":event-type", "contentBlockDelta"),
			want:    "contentBlockDelta",
		},
		{
			name:    "different event",
			headers: buildEventHeaders(":event-type", "metadata"),
			want:    "metadata",
		},
		{
			name:    "empty headers",
			headers: []byte{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEventType(tt.headers)
			if got != tt.want {
				t.Errorf("parseEventType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// buildEventHeaders constructs a header block with a single string header.
func buildEventHeaders(name, value string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(byte(len(name)))
	buf.WriteString(name)
	buf.WriteByte(7) // string type
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(value)))
	buf.WriteString(value)
	return buf.Bytes()
}

func TestModels_noCredentials(t *testing.T) {
	p := &Provider{
		baseURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
		apiKey:  "",
		client:  http.DefaultClient,
	}

	_, err := p.Models(context.Background())
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "missing credentials") {
		t.Errorf("error = %v, want to mention credentials", err)
	}
}

func TestModels_invalidBaseURL(t *testing.T) {
	p := &Provider{
		baseURL: "https://example.com", // no bedrock-runtime
		apiKey:  "test-key",
		client:  http.DefaultClient,
	}

	_, err := p.Models(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if !strings.Contains(err.Error(), "bedrock-runtime") {
		t.Errorf("error = %v, want to mention bedrock-runtime", err)
	}
}

func TestBuildBody_messages(t *testing.T) {
	body := buildBody(provider.Request{
		Messages: []provider.Message{
			{Role: "system", Content: "Be concise"},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "A language."},
			{Role: "user", Content: "Who made it?"},
		},
	})

	// System should be lifted into the dedicated system slot
	if len(body.System) != 1 || body.System[0].Text != "Be concise" {
		t.Errorf("System = %+v, want [{Text: Be concise}]", body.System)
	}
	// Should have 3 messages (system is not in Messages)
	if len(body.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(body.Messages))
	}
	// Verify roles and content
	if body.Messages[0].Role != "user" || body.Messages[0].Content[0].Text != "What is Go?" {
		t.Errorf("Messages[0] = %+v, want user/What is Go?", body.Messages[0])
	}
	if body.Messages[1].Role != "assistant" || body.Messages[1].Content[0].Text != "A language." {
		t.Errorf("Messages[1] = %+v, want assistant/A language.", body.Messages[1])
	}
	if body.Messages[2].Role != "user" || body.Messages[2].Content[0].Text != "Who made it?" {
		t.Errorf("Messages[2] = %+v, want user/Who made it?", body.Messages[2])
	}
}
