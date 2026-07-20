// Package commands implements the cobra command tree for clai.
package commands

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/conversation"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/run"
	"github.com/maxrodrigo/clai/internal/schema"
	"github.com/maxrodrigo/clai/internal/source"
	"github.com/maxrodrigo/clai/internal/strategy"
	"github.com/maxrodrigo/clai/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewRoot creates the root command with all subcommands.
// The out and in parameters provide I/O dependencies for the run pipeline.
func NewRoot(out *output.Output, in *source.Input) *cobra.Command {
	root := &cobra.Command{
		Use:           "clai [flags] <prompt> [files...]",
		Short:         "AI text processing for the UNIX pipeline",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		// TraverseChildren allows flags to be parsed on parent before traversing to children.
		TraverseChildren: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			config.BindFlags(cmd)
			// Resolve color output:
			// fatih/color disables color when stdout is not a TTY, but our
			// diagnostic output (warnings, errors) goes to stderr. Re-enable
			// color when stderr is a terminal, unless --no-color was set.
			switch {
			case config.GetBool("no-color"):
				color.NoColor = true
			case config.GetBool("color"):
				color.NoColor = false
			case color.NoColor:
				// fatih/color disabled color (stdout not a TTY). Check stderr.
				color.NoColor = !term.IsTerminal(int(os.Stderr.Fd()))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := run.PromptOptions{
				InlinePrompt: cmd.Flag("expression").Value.String(),
				PromptFile:   cmd.Flag("file").Value.String(),
				Schema:       config.GetString("schema"),
				DryRun:       config.GetBool("dry-run"),
				Verbose:      config.GetBool("verbose"),
				Conversation: cmd.Flag("conversation").Value.String(),
				ModelFlagSet: cmd.Flags().Changed("model"),
			}

			if opts.InlinePrompt != "" && opts.PromptFile != "" {
				return &UsageError{Msg: "cannot use both -e and -f"}
			}

			// Validate conversation name (reserved tokens pass through).
			if opts.Conversation != "" && opts.Conversation != "-" && opts.Conversation != "+" {
				if err := conversation.ValidateName(opts.Conversation); err != nil {
					return &UsageError{Msg: fmt.Sprintf("invalid conversation name: %v", err)}
				}
			}

			if opts.InlinePrompt == "" && opts.PromptFile == "" {
				if len(args) == 0 && opts.Conversation == "" {
					return cmd.Help()
				}
				if len(args) > 0 {
					opts.PromptName = args[0]
					args = args[1:]
				}
			}

			rt := &run.Runtime{Output: out, Input: in}
			opts.Sources = args // remaining args are file paths
			return run.Prompt(cmd.Context(), rt, opts)
		},
	}

	root.CompletionOptions.HiddenDefaultCmd = true

	root.SetHelpFunc(colorHelpFunc)
	root.ValidArgsFunction = completePromptNames

	root.SetOut(out.Stdout)
	root.SetErr(out.Stderr)

	f := root.PersistentFlags()
	f.StringP("expression", "e", "", "inline prompt text")
	f.StringP("file", "f", "", "read prompt from file")
	f.StringP("model", "m", "", "model (e.g. ollama/llama3.2, openai/gpt-4.1)")
	f.StringP("schema", "s", "", "output schema: shorthand {\"field\":\"type\"} or JSON Schema")
	f.Float64P("temperature", "t", 0, "sampling temperature (omit for model default)")
	f.Int("max-tokens", 0, "maximum tokens to generate")
	f.Bool("think", false, "enable extended thinking (Anthropic/Bedrock only)")
	f.String("strategy", "", "reasoning strategy: cot, cod, tot, self-refine")
	_ = root.RegisterFlagCompletionFunc("strategy", completeStrategyNames)
	f.BoolP("dry-run", "n", false, "print resolved config and prompt without calling the model")
	f.BoolP("verbose", "v", false, "print query details and token counts to stderr")
	f.StringP("conversation", "c", "", "start or continue a conversation: <name>, '-' = most recent, '+' = new auto-named")
	_ = root.RegisterFlagCompletionFunc("conversation", completeConversationNames)
	f.Bool("no-color", false, "disable colored output")
	f.Bool("color", false, "force colored output even when stdout is not a TTY")

	root.AddCommand(
		newPromptCmd(out),
		newStrategyCmd(out),
		newModelCmd(out),
		newConversationCmd(out),
	)

	return root
}

// UsageError indicates the user invoked clai incorrectly.
// Callers can use errors.As to detect this and map it to exit code 2.
type UsageError struct {
	Msg string
}

func (e *UsageError) Error() string { return e.Msg }

// Execute runs the root command and exits with the appropriate code on error.
// Exit codes: 0=success, 1=runtime, 2=usage, 3=schema violation.
func Execute(root *cobra.Command, out *output.Output) {
	if err := root.Execute(); err != nil {
		out.PrintError("clai: %s\n", err)
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps an error to the documented exit code.
func exitCodeFor(err error) int {
	var sv *schema.ValidationError
	if errors.As(err, &sv) {
		return 3
	}

	var ue *UsageError
	if errors.As(err, &ue) {
		return 2
	}

	// Cobra flag-parse errors (unknown flag, missing argument).
	msg := err.Error()
	if strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "flag needs an argument") ||
		strings.Contains(msg, "invalid argument") {
		return 2
	}

	return 1
}

// completePromptNames provides completion for the first positional argument
// (the prompt name). Subsequent args are file paths — cobra's default file
// completion handles those.
func completePromptNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return promptCompletions()
}

// completePromptNamesOnly completes a single prompt name argument.
// Used by subcommands that take exactly one prompt name.
func completePromptNamesOnly(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return promptCompletions()
}

func promptCompletions() ([]string, cobra.ShellCompDirective) {
	prompts, err := prompt.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(prompts))
	for _, p := range prompts {
		names = append(names, p.Name+"\t"+p.Frontmatter.Description)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeStrategyNames provides completion for --strategy flag.
func completeStrategyNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	strategies, err := strategy.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var names []string
	for _, s := range strategies {
		names = append(names, s.Name+"\t"+s.Description)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeStrategyNamesOnly provides completion for subcommands that take a single strategy name.
func completeStrategyNamesOnly(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeStrategyNames(cmd, args, toComplete)
}

// conversationCompletions returns all conversation names for shell completion.
func conversationCompletions() ([]string, cobra.ShellCompDirective) {
	sums, err := conversation.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(sums))
	for _, s := range sums {
		names = append(names, s.Name+"\t"+s.Preview)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeConversationNames completes the -c flag value, including reserved tokens.
func completeConversationNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, dir := conversationCompletions()
	if dir == cobra.ShellCompDirectiveError {
		return nil, dir
	}
	names = append([]string{"-\tmost recent", "+\tnew auto-named"}, names...)
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeConversationNamesOnly completes conversation name arguments for subcommands.
func completeConversationNamesOnly(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return conversationCompletions()
}

// helpSectionRe matches lines like "Usage:", "Available Commands:", "Flags:".
var helpSectionRe = regexp.MustCompile(`(?m)^([A-Z][A-Za-z ]+:)`)

// colorHelpFunc renders cobra's default help with colored section headers.
func colorHelpFunc(cmd *cobra.Command, _ []string) {
	cmd.InitDefaultHelpFlag()

	text := cmd.UsageString()
	if long := cmd.Long; long != "" {
		text = long + "\n\n" + text
	} else if short := cmd.Short; short != "" {
		text = short + "\n\n" + text
	}

	if color.NoColor {
		fmt.Fprint(cmd.OutOrStdout(), text)
		return
	}

	heading := color.New(color.Bold).SprintFunc()
	text = helpSectionRe.ReplaceAllStringFunc(text, func(match string) string {
		return heading(match)
	})

	fmt.Fprint(cmd.OutOrStdout(), text)
}
