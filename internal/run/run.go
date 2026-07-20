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

	// Binary input check: conversation mode requires text-only input.
	if opts.Conversation != "" && input != nil && bytes.ContainsRune(input, 0) {
		return errors.New("binary input not supported in conversation mode")
	}

	systemPrompt, userMessage, err := buildMessages(p, opts, input)
	if err != nil {
		return err
	}

	// --- Conversation resolution ---
	conv, history, err := resolveConversation(rt, opts, userMessage)
	if err != nil {
		return err
	}

	// baseSystem is the undecorated system prompt (stored in conversation).
	// effectiveSystem gets strategy/schema decorations for the API call.
	baseSystem := systemPrompt

	// Inherit system prompt from conversation history if not explicitly provided.
	if conv != nil && systemPrompt == "" {
		if lastSys := conversation.LastSystem(history); lastSys != nil {
			baseSystem = lastSys.Content
			systemPrompt = lastSys.Content
		}
	}

	// Inherit model from conversation history if -m not explicitly set.
	if conv != nil && !opts.ModelFlagSet {
		if inherited := conversation.LastModel(history); inherited != "" {
			cfg.Model = inherited
		}
	}

	// Deferred model check: in conversation mode the model may come from history.
	if cfg.Model == "" {
		return errors.New("no model configured (set -m, CLAI_MODEL, or model in prompt frontmatter)")
	}

	// Apply strategy/schema decorations to the effective system prompt.
	effectiveSystem := systemPrompt
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

	// Multi-turn: build Messages from history.
	if conv != nil {
		req.Messages = buildConversationMessages(effectiveSystem, history, userMessage)
	}

	resp, streamed, err := executeModel(ctx, rt, cfg, req, sch != nil)
	if err != nil {
		if conv != nil && conv.IsNew() {
			cleanupNewConversation(conv)
		}
		return err
	}

	if sch != nil {
		if err := sch.Validate([]byte(resp.Content)); err != nil {
			if conv != nil && conv.IsNew() {
				cleanupNewConversation(conv)
			}
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

	// Persist the turn to the conversation file.
	if conv != nil {
		storeSystem := conv.IsNew() || systemPrompt != ""
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

	// In conversation mode, model may be inherited from history — defer the check.
	if cfg.Model == "" && opts.Conversation == "" {
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
// When all prompt sources are empty (conversation mode without an explicit prompt),
// returns an empty prompt.
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

// resolveConversation resolves the conversation handle and history from opts.
// Returns (nil, nil, nil) for stateless mode (opts.Conversation == "").
func resolveConversation(rt *Runtime, opts PromptOptions, userMessage string) (*conversation.Conversation, []conversation.Message, error) {
	if opts.Conversation == "" {
		return nil, nil, nil
	}

	var conv *conversation.Conversation
	var err error

	switch opts.Conversation {
	case "-":
		conv, err = conversation.Latest()
		if err != nil {
			return nil, nil, err
		}
	case "+":
		conv, err = conversation.New(userMessage)
		if err != nil {
			return nil, nil, err
		}
	default:
		conv, err = conversation.Open(opts.Conversation)
		if err != nil {
			return nil, nil, err
		}
	}

	history, _, err := conv.Messages()
	if err != nil {
		return nil, nil, err
	}

	return conv, history, nil
}

// buildConversationMessages constructs the full multi-turn message list for the API.
// The system prompt is placed first, followed by historical turns, then the new user message.
func buildConversationMessages(system string, history []conversation.Message, user string) []provider.Message {
	var msgs []provider.Message
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

// persistTurn appends the current turn (system if needed, user, assistant) to the conversation file.
func persistTurn(rt *Runtime, conv *conversation.Conversation, storeSystem bool, system, model, user string, resp provider.Response) {
	now := time.Now()
	if storeSystem && system != "" {
		_ = conv.Append(conversation.Message{
			Role:    "system",
			Content: system,
			Model:   model,
			TS:      now,
		})
	}
	_ = conv.Append(conversation.Message{
		Role:    "user",
		Content: user,
		TS:      now,
	})
	_ = conv.Append(conversation.Message{
		Role:      "assistant",
		Content:   resp.Content,
		Model:     model,
		TS:        now,
		TokensIn:  resp.InputTokens,
		TokensOut: resp.OutputTokens,
	})
}

// cleanupNewConversation removes a newly created conversation file on error.
func cleanupNewConversation(conv *conversation.Conversation) {
	_ = os.Remove(conv.Path())
}
