// Package bedrock implements the AWS Bedrock Converse API provider.
package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

func init() {
	provider.Register("bedrock", func(pc config.ProviderConfig) provider.Provider {
		return newProvider(pc)
	})
}

// Provider calls any conversational Bedrock model through the
// Converse API — a unified AWS interface that works with Claude, Nova, Llama,
// Mistral, and all other Bedrock conversational models. Auth uses a bearer
// token (ABSK key from AWS_BEARER_TOKEN_BEDROCK). Set base_url to the Bedrock
// runtime endpoint for your region, e.g.
// https://bedrock-runtime.us-east-1.amazonaws.com.
type Provider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// opErr constructs a provider.OpError for this provider instance.
func (p *Provider) opErr(op string, err error) error {
	return &provider.OpError{Provider: "bedrock", Op: op, Err: err}
}

// errNoCredentials is returned when AWS_BEARER_TOKEN_BEDROCK is not set.
var errNoCredentials = errors.New("missing credentials (set AWS_BEARER_TOKEN_BEDROCK)")

func newProvider(pc config.ProviderConfig) *Provider {
	timeout := 120 * time.Second
	if pc.Timeout > 0 {
		timeout = time.Duration(pc.Timeout) * time.Second
	}
	return &Provider{
		baseURL: strings.TrimRight(pc.BaseURL, "/"),
		apiKey:  pc.APIKey,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "bedrock" }

// defaultThinkBudget is the reasoning token budget used when --think is set
// without an explicit budget. It mirrors the Anthropic provider's default.
const defaultThinkBudget = 10000

// defaultMaxTokens is the response headroom added on top of the reasoning
// budget when --think is enabled and the caller did not request more tokens
// than the budget itself. The Converse API requires maxTokens > budget_tokens.
const defaultMaxTokens = 4096

type converseRequest struct {
	Messages        []converseMessage      `json:"messages"`
	System          []converseTextBlock    `json:"system,omitempty"`
	InferenceConfig *converseInferenceConf `json:"inferenceConfig,omitempty"`
	// AdditionalModelRequestFields carries model-specific parameters that the
	// Converse API passes through, such as Anthropic's reasoning_config.
	AdditionalModelRequestFields map[string]any `json:"additionalModelRequestFields,omitempty"`
}

// reasoningConfig is the additionalModelRequestFields payload that enables
// extended thinking on reasoning-capable Bedrock models (e.g. Claude).
type reasoningConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type converseMessage struct {
	Role    string              `json:"role"`
	Content []converseTextBlock `json:"content"`
}

type converseTextBlock struct {
	Text string `json:"text"`
}

type converseInferenceConf struct {
	MaxTokens   int      `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type converseResponse struct {
	Output struct {
		Message struct {
			Content []converseTextBlock `json:"content"`
		} `json:"message"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	} `json:"usage"`
}

type converseErrorResponse struct {
	Message string `json:"message"`
}

// httpError is returned when Bedrock responds with a non-200 status and no
// JSON body. It implements the statusCoder interface so humanize() can produce
// a user-friendly message.
type httpError struct{ code int }

func (e *httpError) Error() string   { return fmt.Sprintf("HTTP %d", e.code) }
func (e *httpError) StatusCode() int { return e.code }

func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	return p.doRequest(ctx, req, false, nil)
}

func (p *Provider) CompleteStream(ctx context.Context, req provider.Request, w io.Writer) (provider.Response, error) {
	return p.doRequest(ctx, req, true, w)
}

// buildBody assembles a Converse API request body from a provider.Request.
func buildBody(req provider.Request) converseRequest {
	body := converseRequest{
		Messages: []converseMessage{
			{Role: "user", Content: []converseTextBlock{{Text: req.User}}},
		},
	}
	if req.System != "" {
		body.System = []converseTextBlock{{Text: req.System}}
	}

	conf := &converseInferenceConf{}
	if req.MaxTokens > 0 {
		conf.MaxTokens = req.MaxTokens
	}

	// Extended thinking: enable reasoning_config and omit temperature, which
	// reasoning models reject. Ensure maxTokens leaves room beyond the budget.
	if req.Think {
		budget := defaultThinkBudget
		if req.ThinkBudget > 0 {
			budget = req.ThinkBudget
		}
		body.AdditionalModelRequestFields = map[string]any{
			"reasoning_config": reasoningConfig{Type: "enabled", BudgetTokens: budget},
		}
		if conf.MaxTokens <= budget {
			conf.MaxTokens = budget + defaultMaxTokens
		}
	} else if req.Temperature != nil {
		conf.Temperature = req.Temperature
	}

	if conf.MaxTokens != 0 || conf.Temperature != nil {
		body.InferenceConfig = conf
	}
	return body
}

func (p *Provider) doRequest(ctx context.Context, req provider.Request, stream bool, w io.Writer) (provider.Response, error) {
	op := "complete"
	if stream {
		op = "stream"
	}

	if p.apiKey == "" {
		return provider.Response{}, p.opErr(op, errNoCredentials)
	}

	data, err := json.Marshal(buildBody(req))
	if err != nil {
		return provider.Response{}, p.opErr(op, fmt.Errorf("marshaling request: %w", err))
	}

	path := "/model/" + req.Model + "/converse"
	if stream {
		path = "/model/" + req.Model + "/converse-stream"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return provider.Response{}, p.opErr(op, fmt.Errorf("building request: %w", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return provider.Response{}, p.opErr(op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody converseErrorResponse
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errBody); jsonErr != nil || errBody.Message == "" {
			return provider.Response{}, p.opErr(op, &httpError{resp.StatusCode})
		}
		return provider.Response{}, p.opErr(op, errors.New(errBody.Message))
	}

	if stream {
		return p.readStream(resp.Body, w)
	}
	return p.readFull(resp.Body)
}

func (p *Provider) readFull(r io.Reader) (provider.Response, error) {
	var result converseResponse
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return provider.Response{}, p.opErr("complete", fmt.Errorf("decoding response: %w", err))
	}
	var sb strings.Builder
	for _, block := range result.Output.Message.Content {
		sb.WriteString(block.Text)
	}
	return provider.Response{
		Content:      sb.String(),
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}

// readStream parses the AWS EventStream binary framing from the converse-stream
// response, writing text tokens to w as they arrive.
//
// Frame layout (all big-endian):
//
//	[4B total_len][4B headers_len][4B prelude_crc32][headers][payload][4B msg_crc32]
//
// Header entry: [1B name_len][name bytes][1B value_type][2B value_len][value bytes]
// We read :event-type to dispatch, extract text from contentBlockDelta events,
// and token counts from the metadata event.
func (p *Provider) readStream(r io.Reader, w io.Writer) (provider.Response, error) {
	var sb strings.Builder
	var inputTokens, outputTokens int

	for {
		// 12-byte prelude: total_len (4), headers_len (4), prelude_crc (4).
		var prelude [12]byte
		_, err := io.ReadFull(r, prelude[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return provider.Response{}, p.opErr("stream", err)
		}

		totalLen := int(prelude[0])<<24 | int(prelude[1])<<16 | int(prelude[2])<<8 | int(prelude[3])
		headersLen := int(prelude[4])<<24 | int(prelude[5])<<16 | int(prelude[6])<<8 | int(prelude[7])

		// Read the rest of the frame: headers + payload + 4-byte message CRC.
		frameRest := make([]byte, totalLen-12)
		if _, err := io.ReadFull(r, frameRest); err != nil {
			return provider.Response{}, p.opErr("stream", err)
		}

		eventType := parseEventType(frameRest[:headersLen])
		payload := frameRest[headersLen : len(frameRest)-4] // strip trailing CRC

		switch eventType {
		case "contentBlockDelta":
			var ev struct {
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				return provider.Response{}, p.opErr("stream", fmt.Errorf("decoding event: %w", err))
			}
			if ev.Delta.Text != "" {
				sb.WriteString(ev.Delta.Text)
				fmt.Fprint(w, ev.Delta.Text)
			}
		case "metadata":
			var ev struct {
				Usage struct {
					InputTokens  int `json:"inputTokens"`
					OutputTokens int `json:"outputTokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				return provider.Response{}, p.opErr("stream", fmt.Errorf("decoding event: %w", err))
			}
			inputTokens = ev.Usage.InputTokens
			outputTokens = ev.Usage.OutputTokens
		case "internalServerException", "modelStreamErrorException",
			"throttlingException", "validationException":
			var ev struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				return provider.Response{}, p.opErr("stream", errors.New(eventType))
			}
			return provider.Response{}, p.opErr("stream", fmt.Errorf("%s: %s", eventType, ev.Message))
		}
	}

	return provider.Response{
		Content:      sb.String(),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

// parseEventType extracts the :event-type header value from a raw
// EventStream headers block.
//
// Header entry encoding: [1B name_len][name][1B type=7 for string][2B val_len][value]
func parseEventType(headers []byte) string {
	i := 0
	for i < len(headers) {
		nameLen := int(headers[i])
		i++
		if i+nameLen > len(headers) {
			break
		}
		name := string(headers[i : i+nameLen])
		i += nameLen
		if i >= len(headers) {
			break
		}
		i++ // value type byte (7 = string)
		if i+2 > len(headers) {
			break
		}
		valLen := int(headers[i])<<8 | int(headers[i+1])
		i += 2
		if i+valLen > len(headers) {
			break
		}
		val := string(headers[i : i+valLen])
		i += valLen
		if name == ":event-type" {
			return val
		}
	}
	return ""
}

// Models lists models available via Bedrock inference profiles.
// It uses the ListInferenceProfiles API which returns ready-to-use model IDs
// (e.g., "us.anthropic.claude-sonnet-4-20250514-v1:0") that can be passed
// directly to the Converse API without transformation.
func (p *Provider) Models(ctx context.Context) ([]string, error) {
	if p.apiKey == "" {
		return nil, p.opErr("list models", errNoCredentials)
	}

	// The inference-profiles endpoint is on the bedrock control plane
	// (not bedrock-runtime). Derive it by replacing "bedrock-runtime" with "bedrock".
	if !strings.Contains(p.baseURL, "bedrock-runtime") {
		return nil, p.opErr("list models", fmt.Errorf(
			"cannot derive control-plane URL: base_url %q does not contain \"bedrock-runtime\"", p.baseURL,
		))
	}
	controlURL := strings.Replace(p.baseURL, "bedrock-runtime", "bedrock", 1)

	var ids []string
	var nextToken string

	for {
		endpoint := controlURL + "/inference-profiles?maxResults=1000"
		if nextToken != "" {
			endpoint += "&nextToken=" + url.QueryEscape(nextToken)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, p.opErr("list models", fmt.Errorf("building request: %w", err))
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, p.opErr("list models", err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, p.opErr("list models", &httpError{resp.StatusCode})
		}

		var result struct {
			InferenceProfileSummaries []struct {
				InferenceProfileID string `json:"inferenceProfileId"`
				Status             string `json:"status"`
			} `json:"inferenceProfileSummaries"`
			NextToken string `json:"nextToken"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, p.opErr("list models", fmt.Errorf("decoding response: %w", decodeErr))
		}

		for _, profile := range result.InferenceProfileSummaries {
			if profile.Status != "ACTIVE" {
				continue
			}
			ids = append(ids, profile.InferenceProfileID)
		}

		if result.NextToken == "" {
			break
		}
		nextToken = result.NextToken
	}

	return ids, nil
}
