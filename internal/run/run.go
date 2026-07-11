// Package run implements the main execution pipeline for clai.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/provider"
	"github.com/maxrodrigo/clai/internal/schema"
	"github.com/maxrodrigo/clai/internal/source"
	"github.com/maxrodrigo/clai/internal/strategy"
)

// PromptOptions holds the resolved prompt specification and runtime flags.
type PromptOptions struct {
	PromptName   string   // Named prompt (first positional arg)
	InlinePrompt string   // Literal prompt via -e flag
	PromptFile   string   // Prompt file via -f flag
	Sources      []string // Input file paths

	// Runtime flags resolved by the command layer.
	Schema  string // Output schema (from --schema flag)
	DryRun  bool   // Print resolved config without calling the model
	Verbose bool   // Print query details and token counts
}

// Runtime holds the I/O dependencies for the run package.
type Runtime struct {
	Output *output.Output
	Input  *source.Input
}

// Prompt implements the root command: clai [flags] <prompt> [files...].
func Prompt(ctx context.Context, rt *Runtime, opts PromptOptions) error {
	cfg, p, err := resolveConfig(opts)
	if err != nil {
		return err
	}

	schemaStr := opts.Schema
	if schemaStr == "" {
		schemaStr = p.Frontmatter.Schema
	}
	var sch *schema.Schema
	if schemaStr != "" {
		sch, err = schema.Parse(schemaStr)
		if err != nil {
			return err
		}
	}

	var strat *strategy.Strategy
	if cfg.Strategy != "" {
		strat, err = strategy.Resolve(cfg.Strategy)
		if err != nil {
			return err
		}
	}

	input, err := rt.Input.Resolve(opts.Sources, cfg)
	if err != nil {
		return err
	}

	systemPrompt, userMessage, err := buildMessages(p, opts, input)
	if err != nil {
		return err
	}

	if strat != nil {
		systemPrompt = strat.Apply(systemPrompt)
	}
	if sch != nil {
		systemPrompt += "\n\n" + sch.SystemInstruction()
	}

	if opts.DryRun {
		rt.Output.PrintDryRun(cfg.Model, systemPrompt, userMessage)
		return nil
	}

	if opts.Verbose {
		rt.Output.PrintVerbosePre(cfg.Model, systemPrompt, userMessage)
	}

	start := time.Now()

	req := provider.Request{
		Model:       cfg.ModelName(),
		System:      systemPrompt,
		User:        userMessage,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
		Think:       cfg.Think,
		ThinkBudget: cfg.ThinkBudget,
		JSONMode:    sch != nil,
	}

	resp, streamed, err := executeModel(ctx, rt, cfg, req, sch != nil)
	if err != nil {
		return err
	}

	if sch != nil {
		if err := sch.Validate([]byte(resp.Content)); err != nil {
			return err
		}
	}

	switch {
	case !streamed:
		rt.Output.WriteOutput(resp.Content, sch != nil)
	case sch != nil:
		// Streaming already printed raw tokens; re-print pretty JSON on a new line.
		fmt.Fprint(rt.Output.Stdout, "\n")
		rt.Output.WriteOutput(resp.Content, true)
	case resp.Content != "" && resp.Content[len(resp.Content)-1] != '\n':
		fmt.Fprintln(rt.Output.Stdout)
	}

	if opts.Verbose {
		rt.Output.PrintVerbosePost(resp.InputTokens, resp.OutputTokens, time.Since(start).Seconds())
	}

	return nil
}

// resolveConfig handles config loading, prompt resolution, and model override.
func resolveConfig(opts PromptOptions) (*config.Config, *prompt.Prompt, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	p, err := resolvePrompt(opts)
	if err != nil {
		return nil, nil, err
	}

	cfg = prompt.MergeDefaults(cfg, p.Frontmatter)

	// Per-prompt model override: CLAI_MODEL_<PROMPT_NAME> (uppercased, hyphens → underscores).
	// Overrides frontmatter but not explicit user config (config file, CLAI_MODEL, or -m flag).
	// Only applies to named prompts, not inline (-e) or file (-f) prompts.
	if opts.PromptName != "" && !cfg.ModelSet {
		envKey := "CLAI_MODEL_" + promptEnvKey(opts.PromptName)
		if envModel := os.Getenv(envKey); envModel != "" {
			cfg.Model = envModel
		}
	}

	if cfg.Model == "" {
		return nil, nil, errors.New("no model configured (set -m, CLAI_MODEL, or model in prompt frontmatter)")
	}

	return cfg, p, nil
}

// buildMessages constructs the system prompt and user message from the resolved
// prompt and input data.
func buildMessages(p *prompt.Prompt, opts PromptOptions, input []byte) (systemPrompt, userMessage string, err error) {
	if len(input) == 0 {
		if opts.InlinePrompt != "" {
			return "", p.Content, nil
		}
		return "", "", errors.New("no input provided")
	}
	return p.Content, string(input), nil
}

// executeModel handles provider creation, streaming decision, spinner, and model call.
// Returns the response, whether streaming was used, and any error.
func executeModel(ctx context.Context, rt *Runtime, cfg *config.Config, req provider.Request, hasSchema bool) (provider.Response, bool, error) {
	prov, err := provider.Get(cfg.Model, cfg)
	if err != nil {
		return provider.Response{}, false, err
	}

	stream := rt.Output.IsStdoutTerminal() && !hasSchema

	spinner := rt.Output.NewSpinner("thinking")
	var resp provider.Response
	if stream {
		resp, err = prov.CompleteStream(ctx, req, output.NewSpinnerWriter(rt.Output.Stdout, spinner))
	} else {
		resp, err = prov.Complete(ctx, req)
	}
	spinner.Stop()
	if err != nil {
		return provider.Response{}, false, err
	}

	return resp, stream, nil
}

// resolvePrompt resolves the prompt from options (name, -e, or -f).
func resolvePrompt(opts PromptOptions) (*prompt.Prompt, error) {
	switch {
	case opts.InlinePrompt != "":
		return &prompt.Prompt{
			Name:    "(inline)",
			Path:    prompt.LiteralPath,
			Content: opts.InlinePrompt,
		}, nil

	case opts.PromptFile != "":
		data, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("reading prompt file: %w", err)
		}
		return prompt.Parse("(file)", opts.PromptFile, data)

	default:
		return prompt.Resolve(opts.PromptName)
	}
}

// promptEnvKey converts a prompt name to an environment variable suffix.
// Uppercases and replaces hyphens and slashes with underscores:
// "my-prompt" → "MY_PROMPT", "alice/review" → "ALICE_REVIEW".
func promptEnvKey(name string) string {
	s := strings.ReplaceAll(name, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return strings.ToUpper(s)
}
