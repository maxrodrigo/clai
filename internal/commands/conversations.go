package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/maxrodrigo/clai/internal/conversation"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/spf13/cobra"
)

func newConversationCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "conversation",
		Aliases: []string{"conversations"},
		Short:   "Manage conversations",
		Long: `Manage conversations persisted by the -c flag.

Start or continue a conversation with -c on any clai invocation:

  -c <name>   create or continue the named conversation
  -c -        continue the most recent conversation
  -c +        start a new conversation, auto-named from the input

Names are lowercase slugs ([a-z0-9._-], not starting with '-' or '.').
The system prompt and model from the first turn are inherited on
continuation; pass a prompt or -m to override for a turn.

Conversations are JSONL files, one message per line, stored under
$XDG_STATE_HOME/clai/conversations (override with CLAI_CONVERSATIONS_DIR).`,
		Example: `  echo "what is k8s?" | clai -c k8s -e "explain concisely"
  echo "and swarm?"   | clai -c k8s
  echo "more"         | clai -c -
  echo "new topic"    | clai -c +

  clai conversation list
  clai conversation show k8s
  clai conversation rename k8s k8s-basics
  clai conversation remove k8s-basics
  clai conversation remove --older-than 30d`,
	}
	cmd.AddCommand(
		newConversationListCmd(out),
		newConversationShowCmd(out),
		newConversationRenameCmd(out),
		newConversationRemoveCmd(out),
	)
	return cmd
}

func newConversationListCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List conversations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sums, err := conversation.List()
			if err != nil {
				return err
			}
			if len(sums) == 0 {
				fmt.Fprintln(out.Stderr, "no conversations found")
				return nil
			}
			header := color.New(color.Faint)
			name := color.New(color.FgCyan)
			_, _ = header.Fprintf(out.Stdout, "%-20s  %-24s  %-12s  %s\n", "NAME", "MODEL", "UPDATED", "PREVIEW")
			for _, s := range sums {
				updated := s.ModTime.Format("2006-01-02")
				_, _ = name.Fprintf(out.Stdout, "%-20s", s.Name)
				fmt.Fprintf(out.Stdout, "  %-24s  %-12s  %s\n", s.Model, updated, s.Preview)
			}
			return nil
		},
	}
}

func newConversationShowCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print conversation messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := conversation.Open(args[0])
			if err != nil {
				return err
			}
			msgs, _, err := c.Messages()
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				fmt.Fprintln(out.Stderr, "no messages")
				return nil
			}
			role := color.New(color.Faint)
			for _, m := range msgs {
				_, _ = role.Fprintf(out.Stdout, "[%s] ", m.Role)
				fmt.Fprintln(out.Stdout, m.Content)
			}
			return nil
		},
	}
	cmd.ValidArgsFunction = completeConversationNamesOnly
	return cmd
}

func newConversationRenameCmd(out *output.Output) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "Rename a conversation",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := conversation.Rename(args[0], args[1]); err != nil {
				return err
			}
			out.PrintSuccess("renamed %s → %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConversationRemoveCmd(out *output.Output) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [name]",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove a conversation or conversations older than a duration",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			olderThan, _ := cmd.Flags().GetString("older-than")

			if olderThan != "" {
				age, err := parseDuration(olderThan)
				if err != nil {
					return fmt.Errorf("invalid duration %q: %w", olderThan, err)
				}
				n, err := conversation.RemoveOlderThan(age)
				if err != nil {
					return err
				}
				out.PrintSuccess("removed %d conversation(s)\n", n)
				return nil
			}

			if len(args) == 0 {
				return &UsageError{Msg: "provide a conversation name or --older-than"}
			}
			if err := conversation.Remove(args[0]); err != nil {
				return err
			}
			out.PrintSuccess("removed %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("older-than", "", "remove conversations older than duration (e.g. 30d, 720h)")
	cmd.ValidArgsFunction = completeConversationNamesOnly
	return cmd
}

// parseDuration parses a duration string. Supports "Nd" as days shorthand
// in addition to standard Go durations (e.g. "720h").
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
