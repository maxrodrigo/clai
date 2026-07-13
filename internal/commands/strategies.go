package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/strategy"
	"github.com/spf13/cobra"
)

func newStrategyCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "strategy",
		Aliases: []string{"strategies"},
		Short:   "Manage reasoning strategies",
		Long: `Manage reasoning strategies that modify how the AI approaches problems.

Strategies are techniques like Chain-of-Thought (cot), Tree-of-Thought (tot),
and Self-Refine that can improve output quality for complex tasks.`,
		Example: `  clai strategy list
  clai strategy show cot
  clai summarize --strategy cot complex.txt`,
	}
	cmd.AddCommand(
		newStrategyListCmd(out),
		newStrategyShowCmd(out),
		newStrategyPathCmd(out),
	)
	return cmd
}

func newStrategyListCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available strategies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			strategies, err := strategy.List()
			if err != nil {
				return err
			}
			if len(strategies) == 0 {
				fmt.Fprintln(out.Stderr, "no strategies found")
				return nil
			}
			header := color.New(color.Faint)
			name := color.New(color.FgCyan)
			header.Fprintf(out.Stdout, "%-14s  %s\n", "NAME", "DESCRIPTION")
			for _, s := range strategies {
				name.Fprintf(out.Stdout, "%-14s", s.Name)
				fmt.Fprintf(out.Stdout, "  %s\n", s.Description)
			}
			return nil
		},
	}
}

func newStrategyShowCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a strategy's prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := strategy.Resolve(args[0])
			if err != nil {
				return err
			}
			if s == nil {
				return fmt.Errorf("%q is not a strategy", args[0])
			}
			fmt.Fprintln(out.Stdout, s.Prompt)
			return nil
		},
	}
	cmd.ValidArgsFunction = completeStrategyNamesOnly
	return cmd
}

func newStrategyPathCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path <name>",
		Short: "Print the file path of a strategy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := strategy.Resolve(args[0])
			if err != nil {
				return err
			}
			if s == nil {
				return fmt.Errorf("%q is not a strategy", args[0])
			}
			if strategy.IsSystemPath(s.Path) {
				return fmt.Errorf("%q is a system strategy and has no editable file path", args[0])
			}
			fmt.Fprintln(out.Stdout, s.Path)
			return nil
		},
	}
	cmd.ValidArgsFunction = completeStrategyNamesOnly
	return cmd
}
