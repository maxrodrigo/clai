// Package run implements the main execution pipeline for clai.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/conversation"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/provider"
	"github.com/maxrodrigo/clai/internal/schema"
	"github.com/maxrodrigo/clai/internal/source"
	"github.com/maxrodrigo/clai/internal/strategy"
)

// errNoModel is returned when no model is configured via -m, CLAI_MODEL,
// or prompt frontmatter.
var errNoModel = errors.New("no model configured (set -m, CLAI_MODEL, or model in prompt frontmatter)")

// PromptOptions holds the resolved prompt specification and runtime flags.
type PromptOptions struct {
	PromptName   string   // Named prompt (first positional arg)
	InlinePrompt string   // Literal prompt via -e flag
	PromptFile   string   // Prompt file via -f flag
	Sources      []string // Input file paths

	// Runtime flags resolved by the command layer.
	Schema       string // Output schema (from --schema flag)
	DryRun       bool   // Print resolved config without calling the model
	Verbose      bool   // Print query details and token counts
	Conversation string // -c flag value: name, "-", "+", or "" (stateless)
	ModelFlagSet bool   // whether -m was explicitly passed
}

// Runtime holds the I/O dependencies for the run package.
type Runtime struct {
	Output *output.Output
	Input  *source.Input
	// Provider overrides provider lookup for testing. When nil, the provider
	// is resolved from config via provider.Get.
	Provider provider.Provider
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

	if opts.Conversation != "" && bytes.ContainsRune(input, 0) {
		return errors.New("binary input not supported in conversation mode")
	}

	systemPrompt, userMessage, err := buildMessages(p, opts, input)
	if err != nil {
		return err
	}

	conv, history, err := resolveConversation(rt, opts, userMessage)
	if err != nil {
		return err
	}

	// baseSystem is stored in the conversation; effectiveSystem gets
	// strategy/schema decorations for the API call.
	//
	// With no piped input, -e text is the user message (see buildMessages)
	// and must not clobber the stored system prompt.
	explicit := explicitPrompt(opts) && systemPrompt != ""
	baseSystem := systemPrompt

	// Inherit system prompt from history when not explicitly overridden.
	if conv != nil && !explicit {
		if lastSys := conversation.LastSystem(history); lastSys != nil {
			baseSystem = lastSys.Content
		}
	}

	// Inherit model from history when -m not set.
	if conv != nil && !opts.ModelFlagSet {
		if inherited := conversation.LastModel(history); inherited != "" {
			cfg.Model = inherited
		}
	}

	// Model may come from conversation history; check after inheritance.
	if cfg.Model == "" {
		return errNoModel
	}

	effectiveSystem := baseSystem
	if strat != nil {
		effectiveSystem = strat.Apply(effectiveSystem)
	}
	if sch != nil {
		effectiveSystem += "\n\n" + sch.SystemInstruction()
	}

	if opts.DryRun {
		if conv != nil && len(history) > 0 {
			dryMsgs := make([]output.DryRunMessage, 0, len(history))
			for _, m := range history {
				if m.Role == "system" {
					continue
				}
				dryMsgs = append(dryMsgs, output.DryRunMessage{Role: m.Role, Content: m.Content})
			}
			rt.Output.PrintDryRunHistory(dryMsgs)
		}
		rt.Output.PrintDryRun(cfg.Model, effectiveSystem, userMessage)
		return nil
	}

	if opts.Verbose {
		rt.Output.PrintVerbosePre(cfg.Model, effectiveSystem, userMessage)
	}

	start := time.Now()

	req := provider.Request{
		Model:       cfg.ModelName(),
		System:      effectiveSystem,
		User:        userMessage,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
		Think:       cfg.Think,
		ThinkBudget: cfg.ThinkBudget,
		JSONMode:    sch != nil,
	}

	if conv != nil {
		req.Messages = buildConversationMessages(effectiveSystem, history, userMessage)
	}

	resp, streamed, err := executeModel(ctx, rt, cfg, req, sch != nil)
	if err != nil {
		cleanupNewConversation(conv)
		return err
	}

	if sch != nil {
		if err := sch.Validate([]byte(resp.Content)); err != nil {
			cleanupNewConversation(conv)
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

	if conv != nil {
		storeSystem := conv.IsNew() || len(history) == 0 || explicit
		persistTurn(rt, conv, storeSystem, baseSystem, cfg.Model, userMessage, resp)
		if conv.IsNew() {
			rt.Output.PrintHint("[clai] new conversation '%s'\n", conv.Name)
		}
	}

	if opts.Verbose {
		rt.Output.PrintVerbosePost(resp.InputTokens, resp.OutputTokens, time.Since(start).Seconds())
	}

	return nil
}

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

	// In conversation mode, model may be inherited from history — defer the check.
	if cfg.Model == "" && opts.Conversation == "" {
		return nil, nil, errNoModel
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

func executeModel(ctx context.Context, rt *Runtime, cfg *config.Config, req provider.Request, hasSchema bool) (provider.Response, bool, error) {
	prov := rt.Provider
	if prov == nil {
		var err error
		prov, err = provider.Get(cfg.Model, cfg)
		if err != nil {
			return provider.Response{}, false, err
		}
	}

	stream := rt.Output.IsStdoutTerminal() && !hasSchema

	spinner := rt.Output.NewSpinner("thinking")
	var resp provider.Response
	var err error
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
// Returns an empty prompt when all sources are empty (conversation mode).
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

	case opts.PromptName != "":
		return prompt.Resolve(opts.PromptName)

	default:
		// No prompt specified — valid in conversation mode.
		return &prompt.Prompt{}, nil
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

// Returns (nil, nil, nil) for stateless mode.
func resolveConversation(rt *Runtime, opts PromptOptions, userMessage string) (*conversation.Conversation, []conversation.Message, error) {
	if opts.Conversation == "" {
		return nil, nil, nil
	}

	var conv *conversation.Conversation
	var err error

	switch opts.Conversation {
	case "-":
		conv, err = conversation.Latest()
	case "+":
		if opts.DryRun {
			return nil, nil, nil
		}
		conv, err = conversation.New(userMessage)
	default:
		conv, err = conversation.Open(opts.Conversation)
	}
	if err != nil {
		return nil, nil, err
	}

	history, skipped, err := conv.Messages()
	if err != nil {
		return nil, nil, err
	}
	if skipped > 0 {
		rt.Output.PrintWarning("warning: skipped %d malformed line(s) in conversation '%s'\n", skipped, conv.Name)
	}

	return conv, history, nil
}

// buildConversationMessages constructs the multi-turn message list for the API.
func buildConversationMessages(system string, history []conversation.Message, user string) []provider.Message {
	msgs := make([]provider.Message, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, provider.Message{Role: "system", Content: system})
	}
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, provider.Message{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, provider.Message{Role: "user", Content: user})
	return msgs
}

// persistTurn appends this turn to the conversation. Failures are warnings,
// not errors — the response has already been delivered on stdout.
func persistTurn(rt *Runtime, conv *conversation.Conversation, storeSystem bool, system, model, user string, resp provider.Response) {
	now := time.Now().UTC()
	pending := make([]conversation.Message, 0, 3)
	if storeSystem {
		pending = append(pending, conversation.Message{Role: "system", Content: system, Model: model, TS: now})
	}
	pending = append(pending,
		conversation.Message{Role: "user", Content: user, TS: now},
		conversation.Message{Role: "assistant", Content: resp.Content, TS: now, TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
	)
	for _, m := range pending {
		if err := conv.Append(m); err != nil {
			rt.Output.PrintWarning("warning: conversation not saved: %s\n", err)
			return
		}
	}
}

// cleanupNewConversation removes the empty file left behind when a new
// conversation's first model call fails.
func cleanupNewConversation(conv *conversation.Conversation) {
	if conv == nil || !conv.IsNew() {
		return
	}
	info, err := os.Stat(conv.Path())
	if err != nil || info.Size() > 0 {
		return
	}
	_ = os.Remove(conv.Path())
}

// explicitPrompt reports whether the user provided a prompt on this invocation.
func explicitPrompt(opts PromptOptions) bool {
	return opts.PromptName != "" || opts.InlinePrompt != "" || opts.PromptFile != ""
}
