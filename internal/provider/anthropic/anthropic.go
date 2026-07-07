// Package anthropic implements the Anthropic provider.
package anthropic

import (
	"context"
	"fmt"
	"io"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

func init() {
	provider.Register("anthropic", func(pc config.ProviderConfig) provider.Provider {
		return newProvider(pc)
	})
}

// defaultMaxTokens is used when the caller does not specify MaxTokens.
// Claude 3/4 models support at least 8192 output tokens; newer models support
// more, but 8192 is a safe conservative default across the Anthropic model range.
const defaultMaxTokens = 8192

// defaultThinkBudget is the default token budget for extended thinking when
// the caller enables Think mode but does not specify ThinkBudget.
const defaultThinkBudget = 10000

// Provider wraps the official anthropic-sdk-go SDK.
type Provider struct {
	client anthropicsdk.Client
}

// opErr constructs a provider.OpError for this provider instance.
func (p *Provider) opErr(op string, err error) error {
	return &provider.OpError{Provider: "anthropic", Op: op, Err: err}
}

func newProvider(pc config.ProviderConfig) *Provider {
	opts := []anthropicoption.RequestOption{
		anthropicoption.WithBaseURL(pc.BaseURL),
	}
	if pc.APIKey != "" {
		opts = append(opts, anthropicoption.WithAPIKey(pc.APIKey))
	}
	return &Provider{client: anthropicsdk.NewClient(opts...)}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "anthropic" }

// Complete sends a request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	params := p.buildParams(req)
	return p.completeFull(ctx, params)
}

// CompleteStream sends a request and streams tokens to w as they arrive.
func (p *Provider) CompleteStream(ctx context.Context, req provider.Request, w io.Writer) (provider.Response, error) {
	params := p.buildParams(req)
	return p.completeStreaming(ctx, params, w)
}

func (p *Provider) buildParams(req provider.Request) anthropicsdk.MessageNewParams {
	maxTokens := int64(defaultMaxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}
	params := anthropicsdk.MessageNewParams{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(req.User))},
	}
	if req.System != "" {
		params.System = []anthropicsdk.TextBlockParam{{Text: req.System}}
	}
	if req.Temperature != nil {
		params.Temperature = anthropicsdk.Float(*req.Temperature)
	}
	if req.Think {
		budget := int64(defaultThinkBudget)
		if req.ThinkBudget > 0 {
			budget = int64(req.ThinkBudget)
		}
		params.Thinking = anthropicsdk.ThinkingConfigParamOfEnabled(budget)
	}
	return params
}

func (p *Provider) completeFull(ctx context.Context, params anthropicsdk.MessageNewParams) (provider.Response, error) {
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return provider.Response{}, p.opErr("complete", err)
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropicsdk.TextBlock); ok {
			sb.WriteString(text.Text)
		}
	}
	return provider.Response{
		Content:      sb.String(),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}, nil
}

func (p *Provider) completeStreaming(ctx context.Context, params anthropicsdk.MessageNewParams, w io.Writer) (provider.Response, error) {
	stream := p.client.Messages.NewStreaming(ctx, params)
	var acc anthropicsdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := acc.Accumulate(event); err != nil {
			return provider.Response{}, p.opErr("stream", err)
		}
		if delta, ok := event.AsAny().(anthropicsdk.ContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.AsAny().(anthropicsdk.TextDelta); ok {
				fmt.Fprint(w, text.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return provider.Response{}, p.opErr("stream", err)
	}
	var sb strings.Builder
	for _, block := range acc.Content {
		if text, ok := block.AsAny().(anthropicsdk.TextBlock); ok {
			sb.WriteString(text.Text)
		}
	}
	return provider.Response{
		Content:      sb.String(),
		InputTokens:  int(acc.Usage.InputTokens),
		OutputTokens: int(acc.Usage.OutputTokens),
	}, nil
}

func (p *Provider) Models(ctx context.Context) ([]string, error) {
	iter := p.client.Models.ListAutoPaging(ctx, anthropicsdk.ModelListParams{})
	var ids []string
	for iter.Next() {
		ids = append(ids, iter.Current().ID)
	}
	if err := iter.Err(); err != nil {
		return nil, p.opErr("list models", err)
	}
	return ids, nil
}
