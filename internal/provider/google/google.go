// Package google implements the Gemini and Vertex AI providers using the
// unified google.golang.org/genai SDK.
package google

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/genai"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/provider"
)

func init() {
	provider.Register("gemini", func(pc config.ProviderConfig) provider.Provider {
		return newProvider("gemini", pc)
	})
	provider.Register("vertex", func(pc config.ProviderConfig) provider.Provider {
		return newProvider("vertex", pc)
	})
}

// Provider wraps the google.golang.org/genai SDK client.
// If client construction failed (e.g. missing ADC credentials for Vertex),
// initErr is set and returned on first use — mirroring how bedrock defers
// credential errors to call time.
type Provider struct {
	name    string
	client  *genai.Client
	initErr error
}

func newProvider(name string, pc config.ProviderConfig) *Provider {
	cc := &genai.ClientConfig{}

	switch name {
	case "vertex":
		cc.Backend = genai.BackendVertexAI
		cc.Project = pc.Project
		cc.Location = pc.Location
	default:
		cc.Backend = genai.BackendGeminiAPI
		cc.APIKey = pc.APIKey
	}

	client, err := genai.NewClient(context.Background(), cc)
	return &Provider{name: name, client: client, initErr: err}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) opErr(op string, err error) error {
	return &provider.OpError{Provider: p.name, Op: op, Err: err}
}

func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if p.initErr != nil {
		return provider.Response{}, p.opErr("complete", p.initErr)
	}
	contents := []*genai.Content{genai.NewContentFromText(req.User, genai.RoleUser)}
	cfg := p.buildConfig(req)

	result, err := p.client.Models.GenerateContent(ctx, req.Model, contents, cfg)
	if err != nil {
		return provider.Response{}, p.opErr("complete", err)
	}
	return p.extractResponse(result), nil
}

func (p *Provider) CompleteStream(ctx context.Context, req provider.Request, w io.Writer) (provider.Response, error) {
	if p.initErr != nil {
		return provider.Response{}, p.opErr("stream", p.initErr)
	}
	contents := []*genai.Content{genai.NewContentFromText(req.User, genai.RoleUser)}
	cfg := p.buildConfig(req)

	var sb strings.Builder
	var resp provider.Response

	for result, err := range p.client.Models.GenerateContentStream(ctx, req.Model, contents, cfg) {
		if err != nil {
			return provider.Response{}, p.opErr("stream", err)
		}
		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			for _, part := range result.Candidates[0].Content.Parts {
				sb.WriteString(part.Text)
				fmt.Fprint(w, part.Text)
			}
		}
		if result.UsageMetadata != nil {
			resp.InputTokens = int(result.UsageMetadata.PromptTokenCount)
			resp.OutputTokens = int(result.UsageMetadata.CandidatesTokenCount)
		}
	}
	resp.Content = sb.String()
	return resp, nil
}

func (p *Provider) Models(ctx context.Context) ([]string, error) {
	if p.initErr != nil {
		return nil, p.opErr("list models", p.initErr)
	}
	var ids []string
	for model, err := range p.client.Models.All(ctx) {
		if err != nil {
			return nil, p.opErr("list models", err)
		}
		// The SDK returns names like "models/gemini-2.5-flash"; strip the
		// prefix so users can reference them as "gemini/gemini-2.5-flash".
		name := strings.TrimPrefix(model.Name, "models/")
		ids = append(ids, name)
	}
	return ids, nil
}

func (p *Provider) buildConfig(req provider.Request) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}

	if req.System != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: req.System}}}
	}
	if req.Temperature != nil {
		cfg.Temperature = genai.Ptr[float32](float32(*req.Temperature))
	}
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	}
	if req.JSONMode {
		cfg.ResponseMIMEType = "application/json"
	}
	if req.Think {
		budget := int32(defaultThinkBudget)
		if req.ThinkBudget > 0 {
			budget = int32(req.ThinkBudget)
		}
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingBudget: genai.Ptr[int32](budget),
		}
	}
	return cfg
}

func (p *Provider) extractResponse(result *genai.GenerateContentResponse) provider.Response {
	resp := provider.Response{
		Content: result.Text(),
	}
	if result.UsageMetadata != nil {
		resp.InputTokens = int(result.UsageMetadata.PromptTokenCount)
		resp.OutputTokens = int(result.UsageMetadata.CandidatesTokenCount)
	}
	return resp
}

// defaultThinkBudget is the token budget for thinking when the caller enables
// Think mode but does not specify ThinkBudget.
const defaultThinkBudget = 10000
