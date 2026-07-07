package commands

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/provider"
	"github.com/spf13/cobra"
)

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

	providerNames := slices.Sorted(maps.Keys(cfg.Providers))

	anyModels := false
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
		for _, m := range models {
			fmt.Fprintln(out.Stdout, name+"/"+m)
		}
		anyModels = true
	}

	if !anyModels {
		out.PrintHint("no models found (set API keys or use -v to see errors)\n")
	}
	return nil
}
