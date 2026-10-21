package comments

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/commentfmt"
	"github.com/spf13/cobra"
)

// maxReplyStdin bounds a reply read from stdin; the API caps a body at
// 5000 characters, i.e. at most 20000 bytes of UTF-8.
const maxReplyStdin = 64 << 10

func newReplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply <egg> <comment-id> <text>",
		Short: "Reply to a comment thread as an agent (or --owner)",
		Long: "Reply to the thread containing <comment-id>. Pass - as <text> to read it from stdin.\n" +
			"The reply shows as \"Agent · <label>\"; --as sets the label (default: your token's name).\n" +
			"--owner posts as yourself instead of as an agent.",
		Args: cobra.ExactArgs(3),
		RunE: runReply,
	}
	cmd.Flags().String("as", "", "Agent label shown with the reply (e.g. \"Claude (bosco)\")")
	cmd.Flags().Bool("owner", false, "Reply as the egg's owner rather than as an agent")
	cmd.MarkFlagsMutuallyExclusive("as", "owner")
	return cmd
}

func runReply(cmd *cobra.Command, args []string) error {
	slug, id, text := args[0], args[1], args[2]
	if err := api.ValidateCommentID(id); err != nil {
		return err
	}
	if text == "-" {
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxReplyStdin+1))
		if err != nil {
			return fmt.Errorf("reading reply from stdin: %w", err)
		}
		if len(data) > maxReplyStdin {
			return errors.New("reply on stdin is too long (a comment is at most 5000 characters)")
		}
		text = string(data)
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("the reply text is empty")
	}

	nc := api.NewComment{ParentID: id, Body: text}
	nc.AuthorLabel, _ = cmd.Flags().GetString("as")
	if owner, _ := cmd.Flags().GetBool("owner"); owner {
		nc.AuthorKind = "owner"
	}

	c, err := client()
	if err != nil {
		return err
	}
	reply, err := c.CreateComment(slug, nc)
	if err != nil {
		return apiErr("replying", slug, err)
	}
	thread := id
	if reply.ParentID != nil {
		thread = *reply.ParentID
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Replied to thread %s on %s as %s (comment %s).\n",
		thread, slug, commentfmt.Author(*reply), reply.ID)
	return nil
}

func newThreadActionCmd(verb, short, done string, call func(APIClient, string, string) (*api.Comment, error)) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <egg> <comment-id>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, id := args[0], args[1]
			if err := api.ValidateCommentID(id); err != nil {
				return err
			}
			c, err := client()
			if err != nil {
				return err
			}
			res, err := call(c, slug, id)
			if err != nil {
				return apiErr(verb+" failed", slug, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), done+"\n", res.ID, slug)
			return nil
		},
	}
}

func newResolveCmd() *cobra.Command {
	return newThreadActionCmd("resolve", "Resolve the thread containing a comment",
		"Resolved thread %s on %s.", APIClient.ResolveComment)
}

func newReopenCmd() *cobra.Command {
	return newThreadActionCmd("reopen", "Reopen the thread containing a comment",
		"Reopened thread %s on %s.", APIClient.ReopenComment)
}

func newDeleteCmd() *cobra.Command {
	return newThreadActionCmd("delete", "Delete a comment (moderation; kept in history until purged)",
		"Deleted comment %s on %s.", APIClient.DeleteComment)
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <egg>",
		Short: "Export every stored comment as JSON (deleted ones included)",
		Long: "Export every stored comment, oldest first, as the API's JSON dump. The dump holds " +
			"visitors' emails and texts (personal data): -o writes it with owner-only permissions.",
		Args: cobra.ExactArgs(1),
		RunE: runExport,
	}
	cmd.Flags().StringP("output", "o", "", "Write to this file instead of stdout")
	return cmd
}

func runExport(cmd *cobra.Command, args []string) error {
	slug := args[0]
	c, err := client()
	if err != nil {
		return err
	}
	data, err := c.ExportComments(slug)
	if err != nil {
		return apiErr("exporting comments", slug, err)
	}
	path, _ := cmd.Flags().GetString("output")
	if path == "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}
	if err := deps.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Exported comments of %s to %s.\n", slug, path)
	return nil
}

func newPurgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge <egg> (--author EMAIL | --before DATE | --all)",
		Short: "Permanently erase comments (by author, by date, or all)",
		Long: "Permanently erase comments — no undo, not even in export. A purged thread takes its " +
			"replies with it. --author and --before can be combined; --all erases everything.\n" +
			"--author also clears that person's email from the \"resolved by\" / \"deleted by\" " +
			"marks on every remaining comment of the egg, whatever --before says (right to erasure).\n" +
			"--before takes a date (2026-10-18), an RFC 3339 time, or a duration (30d = older than 30 days).\n" +
			"Asks you to type the egg name unless --yes; without a terminal, --yes is required.",
		Args: cobra.ExactArgs(1),
		RunE: runPurge,
	}
	cmd.Flags().String("author", "", "Erase every comment written by this email")
	cmd.Flags().String("before", "", "Erase comments created before this date, time or duration")
	cmd.Flags().Bool("all", false, "Erase every comment of the egg")
	cmd.Flags().BoolP("yes", "y", false, "Skip the confirmation prompt")
	cmd.MarkFlagsMutuallyExclusive("all", "author")
	cmd.MarkFlagsMutuallyExclusive("all", "before")
	cmd.MarkFlagsOneRequired("author", "before", "all")
	return cmd
}

func runPurge(cmd *cobra.Command, args []string) error {
	slug := args[0]
	author, _ := cmd.Flags().GetString("author")
	beforeArg, _ := cmd.Flags().GetString("before")
	all, _ := cmd.Flags().GetBool("all")
	yes, _ := cmd.Flags().GetBool("yes")

	p := api.PurgeComments{AuthorEmail: strings.ToLower(strings.TrimSpace(author)), All: all}
	if strings.TrimSpace(beforeArg) != "" {
		before, err := commentfmt.ParseSince(beforeArg, deps.Now())
		if err != nil {
			return errors.New(strings.Replace(err.Error(), "invalid since", "invalid before", 1))
		}
		p.Before = before
	}
	if !p.All && p.AuthorEmail == "" && p.Before == "" {
		return errors.New("specify --author, --before or --all")
	}

	if !yes {
		if !deps.StdinIsTerminal() {
			return errors.New("refusing to purge without --yes: stdin is not a terminal")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "This permanently erases %s on %s. There is no undo.\n", purgeScope(p), slug)
		fmt.Fprintf(cmd.ErrOrStderr(), "Type %q to confirm: ", slug)
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if strings.TrimSpace(answer) != slug {
			fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled. Egg name did not match.")
			return nil
		}
	}

	c, err := client()
	if err != nil {
		return err
	}
	n, err := c.PurgeComments(slug, p)
	if err != nil {
		return apiErr("purging comments", slug, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Purged %d comment(s) on %s.\n", n, slug)
	return nil
}

func purgeScope(p api.PurgeComments) string {
	if p.All {
		return "every comment"
	}
	var parts []string
	if p.AuthorEmail != "" {
		parts = append(parts, "by "+p.AuthorEmail)
	}
	if p.Before != "" {
		parts = append(parts, "created before "+p.Before)
	}
	return "the comments " + strings.Join(parts, " and ")
}
