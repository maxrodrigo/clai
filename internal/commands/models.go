package commands

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"

	"github.com/fatih/color"
	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/provider"
	"github.com/spf13/cobra"
)

// providerColor returns a deterministic color for a provider name.
// Uses 12 distinct ANSI foreground colors (6 standard + 6 hi-intensity),
// excluding black and white for readability on both light and dark terminals.
func providerColor(name string) *color.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name)) // hash.Hash.Write never returns an error
	idx := h.Sum32() % 12
	if idx < 6 {
		return color.New(color.Attribute(31 + idx)) // FgRed(31) through FgCyan(36)
	}
	return color.New(color.Attribute(91 + idx - 6)) // FgHiRed(91) through FgHiCyan(96)
}

func newModelCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "model",
		Aliases: []string{"models"},
		Short:   "Manage models",
		Long: `Manage models available from configured providers.

Models are grouped by provider and shown in provider/model format.
Use -v to see warnings when a provider fails to respond.`,
		Example: `  clai model list
  clai model list | grep gpt
  clai model list -v`,
	}
	cmd.AddCommand(
		newModelListCmd(out),
	)
	return cmd
}

func newModelListCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available models from configured providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			return listModels(cmd.Context(), out, verbose)
		},
	}
	return cmd
}

func listModels(ctx context.Context, out *output.Output, verbose bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	header := color.New(color.Faint)
	providerNames := slices.Sorted(maps.Keys(cfg.Providers))

	anyModels := false
	headerPrinted := false
	for _, name := range providerNames {
		prov, err := provider.GetByName(name, cfg)
		if err != nil {
			if verbose {
				out.PrintWarning("warning: %s: %v\n", name, err)
			}
			continue
		}
		models, err := prov.Models(ctx)
		if err != nil {
			if verbose {
				out.PrintWarning("warning: %s: %v\n", name, err)
			}
			continue
		}
		if len(models) == 0 {
			continue
		}
		slices.Sort(models)
		if !headerPrinted {
			_, _ = header.Fprintln(out.Stdout, "MODEL")
			headerPrinted = true
		}
		c := providerColor(name).SprintFunc()
		for _, m := range models {
			fmt.Fprintf(out.Stdout, "%s/%s\n", c(name), m)
		}
		anyModels = true
	}

	if !anyModels {
		out.PrintHint("no models found (set API keys or use -v to see errors)\n")
	}
	return nil
}
