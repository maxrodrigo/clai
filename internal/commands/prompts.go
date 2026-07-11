package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/spf13/cobra"
)

func newPromptCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "prompt",
		Aliases: []string{"prompts"},
		Short:   "Manage prompts",
		Long: `Manage prompt files used by clai.

Prompts are markdown files that define system instructions for the AI model.
They can include YAML frontmatter for default settings like model and temperature.

Community prompts are namespaced as owner/name and managed via 'clai prompt install'.`,
		Example: `  clai prompt list
  clai prompt show summarize
  clai prompt add my-prompt
  clai prompt install alice/review ./review.md
  clai prompt path`,
	}
	cmd.AddCommand(
		newPromptListCmd(out),
		newPromptShowCmd(out),
		newPromptPathCmd(out),
		newPromptAddCmd(out),
		newPromptUpdateCmd(out),
		newPromptRemoveCmd(out),
		newPromptInstallCmd(out),
	)
	return cmd
}

func newPromptListCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available prompts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			groups, err := prompt.ListBySource()
			if err != nil {
				return err
			}
			if len(groups) == 0 {
				fmt.Fprintln(out.Stderr, "no prompts found")
				return nil
			}
			for _, g := range groups {
				for _, p := range g.Prompts {
					desc := p.Frontmatter.Description
					if desc == "" {
						desc = "-"
					}
					fmt.Fprintf(out.Stdout, "%-24s  %-10s  %s\n", p.Name, g.Source, desc)
				}
			}
			return nil
		},
	}
}

func newPromptShowCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a prompt's content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := prompt.Resolve(args[0])
			if err != nil {
				return err
			}
			if p.Frontmatter.Description != "" {
				fmt.Fprintf(out.Stdout, "# %s\n\n", p.Frontmatter.Description)
			}
			fmt.Fprintln(out.Stdout, p.Content)
			return nil
		},
	}
	cmd.ValidArgsFunction = completePromptNamesOnly
	return cmd
}

func newPromptPathCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path [name]",
		Short: "Print the path to a prompt file, or the prompts directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(out.Stdout, filepath.Join(config.Dir(), "prompts"))
				return nil
			}
			p, err := prompt.Resolve(args[0])
			if err != nil {
				return err
			}
			if prompt.IsSystemPath(p.Path) {
				return fmt.Errorf("%q is a system prompt and has no editable file path", args[0])
			}
			fmt.Fprintln(out.Stdout, p.Path)
			return nil
		},
	}
	cmd.ValidArgsFunction = completePromptNamesOnly
	return cmd
}

func newPromptAddCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new prompt and open it in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !prompt.IsValidName(name) {
				return errors.New("prompt name must be alphanumeric with hyphens or underscores")
			}
			dir := filepath.Join(config.Dir(), "prompts")
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return fmt.Errorf("creating prompts directory: %w", err)
			}
			path := filepath.Join(dir, name+".md")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("prompt %q already exists at %s", name, path)
			}
			content := fmt.Sprintf("---\ndescription: %s\n---\nYou are a helpful assistant.\n", name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				return fmt.Errorf("writing prompt: %w", err)
			}
			out.PrintSuccess("created %s\n", path)
			return openInEditor(cmd.Context(), path, out)
		},
	}
}

func newPromptUpdateCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <name>",
		Aliases: []string{"edit"},
		Short:   "Open a prompt in $EDITOR",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := prompt.Resolve(args[0])
			if err != nil {
				return err
			}
			if prompt.IsSystemPath(p.Path) {
				return fmt.Errorf(
					"cannot edit system prompt %q — copy it first:\n  clai prompt show %s > ~/.config/clai/prompts/%s.md",
					args[0], args[0], args[0],
				)
			}
			return openInEditor(cmd.Context(), p.Path, out)
		},
	}
	cmd.ValidArgsFunction = completePromptNamesOnly
	return cmd
}

func newPromptRemoveCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm", "delete"},
		Short:   "Delete a prompt file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := prompt.Resolve(args[0])
			if err != nil {
				return err
			}
			if prompt.IsSystemPath(p.Path) {
				return fmt.Errorf("cannot remove system prompt %q", args[0])
			}
			if err := os.Remove(p.Path); err != nil {
				return fmt.Errorf("removing prompt: %w", err)
			}
			out.PrintSuccess("removed %s\n", p.Path)
			return nil
		},
	}
	cmd.ValidArgsFunction = completePromptNamesOnly
	return cmd
}

// newPromptInstallCmd installs a community prompt (owner/name).
// Copies a local file into ~/.config/clai/community/owner/name.md.
func newPromptInstallCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:     "install <owner/name> <file>",
		Short:   "Install a community prompt",
		Example: `  clai prompt install alice/review ./review.md`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !prompt.IsNamespaced(name) {
				return fmt.Errorf("%q must be in owner/name format", name)
			}

			owner, promptName := prompt.ParseNamespace(name)
			communityDir := filepath.Join(config.Dir(), "community")
			ownerDir := filepath.Join(communityDir, owner)
			promptPath := filepath.Clean(filepath.Join(ownerDir, promptName+".md")) //nolint:gosec // owner/promptName validated by IsNamespaced (alphanum+hyphen only)

			data, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("reading %s: %w", args[1], err)
			}
			if _, err := prompt.Parse(name, promptPath, data); err != nil {
				return fmt.Errorf("invalid prompt: %w", err)
			}
			if err := os.MkdirAll(ownerDir, 0o750); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}
			if err := os.WriteFile(promptPath, data, 0o600); err != nil { //nolint:gosec // path validated by IsNamespaced (no separators in segments)
				return fmt.Errorf("writing prompt: %w", err)
			}
			out.PrintSuccess("installed %s\n", name)
			return nil
		},
	}
}

// openInEditor opens a file in $VISUAL or $EDITOR, falling back to common editors.
func openInEditor(ctx context.Context, path string, out *output.Output) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		for _, e := range []string{"nvim", "vim", "nano", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		out.PrintHint("hint: set $EDITOR to open %s automatically\n", path)
		return nil
	}
	c := exec.CommandContext(ctx, editor, path) //nolint:gosec // editor is from $EDITOR
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
