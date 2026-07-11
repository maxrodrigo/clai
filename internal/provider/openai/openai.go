// Package openai implements the OpenAI provider.
package openai

import (
	"context"
	"fmt"
	"io"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func init() {
	provider.Register("openai", func(pc config.ProviderConfig) provider.Provider {
		return newProvider("openai", pc)
	})
	provider.Register("ollama", func(pc config.ProviderConfig) provider.Provider {
		return newProvider("ollama", pc)
	})
}

// Provider wraps the official openai-go SDK.
type Provider struct {
	name   string
	client openaisdk.Client
}

// opErr constructs a provider.OpError for this provider instance.
func (p *Provider) opErr(op string, err error) error {
	return &provider.OpError{Provider: p.name, Op: op, Err: err}
}

func newProvider(name string, pc config.ProviderConfig) *Provider {
	opts := []option.RequestOption{
		option.WithBaseURL(pc.BaseURL),
	}
	if pc.APIKey != "" {
		opts = append(opts, option.WithAPIKey(pc.APIKey))
	}
	return &Provider{name: name, client: openaisdk.NewClient(opts...)}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return p.name }

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

func (p *Provider) buildParams(req provider.Request) openaisdk.ChatCompletionNewParams {
	var messages []openaisdk.ChatCompletionMessageParamUnion
	if req.System != "" {
		messages = append(messages, openaisdk.SystemMessage(req.System))
	}
	messages = append(messages, openaisdk.UserMessage(req.User))

	params := openaisdk.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: messages,
	}
	if req.MaxTokens > 0 {
		params.MaxTokens = openaisdk.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openaisdk.Float(*req.Temperature)
	}
	if req.JSONMode {
		params.ResponseFormat = openaisdk.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openaisdk.ResponseFormatJSONObjectParam{},
		}
	}
	return params
}

func (p *Provider) completeFull(ctx context.Context, params openaisdk.ChatCompletionNewParams) (provider.Response, error) {
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return provider.Response{}, p.opErr("complete", err)
	}
	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	return provider.Response{
		Content:      content,
		InputTokens:  int(resp.Usage.PromptTokens),
		OutputTokens: int(resp.Usage.CompletionTokens),
	}, nil
}

func (p *Provider) completeStreaming(ctx context.Context, params openaisdk.ChatCompletionNewParams, w io.Writer) (provider.Response, error) {
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openaisdk.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) > 0 {
			fmt.Fprint(w, chunk.Choices[0].Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		return provider.Response{}, p.opErr("stream", err)
	}
	content := ""
	if len(acc.Choices) > 0 {
		content = acc.Choices[0].Message.Content
	}
	return provider.Response{
		Content:      content,
		InputTokens:  int(acc.Usage.PromptTokens),
		OutputTokens: int(acc.Usage.CompletionTokens),
	}, nil
}

func (p *Provider) Models(ctx context.Context) ([]string, error) {
	iter := p.client.Models.ListAutoPaging(ctx)
	var ids []string
	for iter.Next() {
		ids = append(ids, iter.Current().ID)
	}
	if err := iter.Err(); err != nil {
		return nil, p.opErr("list models", err)
	}
	return ids, nil
}
