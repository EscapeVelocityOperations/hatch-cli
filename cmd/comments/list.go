package comments

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/commentfmt"
	"github.com/spf13/cobra"
)

var statusFlags = []string{"open", "resolved", "deleted", "all"}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <egg>",
		Short: "List comment threads as a markdown report (or --json)",
		Long: "List comment threads, oldest first, grouped by page. Defaults to open threads.\n" +
			"--since takes a date (2026-10-18), an RFC 3339 time, or a duration (90m, 24h, 7d).\n" +
			"--json prints a stable document: {egg, status, notice, untrusted_content: {threads}}.",
		Args: cobra.ExactArgs(1),
		RunE: runList,
	}
	cmd.Flags().String("page", "", "Only threads on this page path (e.g. /pricing/)")
	cmd.Flags().Bool("open", false, "Open threads (default)")
	cmd.Flags().Bool("resolved", false, "Resolved threads")
	cmd.Flags().Bool("deleted", false, "Deleted threads")
	cmd.Flags().Bool("all", false, "All threads, with deleted replies")
	cmd.MarkFlagsMutuallyExclusive(statusFlags...)
	cmd.Flags().String("since", "", "Only threads created since this date, time or duration")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	slug := args[0]
	status := "open"
	for _, s := range statusFlags {
		if on, _ := cmd.Flags().GetBool(s); on {
			status = s
		}
	}
	page, _ := cmd.Flags().GetString("page")
	sinceArg, _ := cmd.Flags().GetString("since")
	since, err := commentfmt.ParseSince(sinceArg, deps.Now())
	if err != nil {
		return err
	}

	c, err := client()
	if err != nil {
		return err
	}
	threads, err := c.ListAllComments(slug, api.ListCommentsOptions{Page: page, Status: status, Since: since})
	if err != nil {
		return apiErr("listing comments", slug, err)
	}

	out := cmd.OutOrStdout()
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		data, err := commentfmt.ThreadsAsJSON(slug, status, threads)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	}
	_, err = fmt.Fprint(out, commentfmt.ThreadsMarkdown(slug, status, threads))
	return err
}

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <egg> <comment-id>",
		Short: "Show the full thread containing a comment (deleted replies included)",
		Args:  cobra.ExactArgs(2),
		RunE:  runShow,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func runShow(cmd *cobra.Command, args []string) error {
	slug, id := args[0], args[1]
	if err := api.ValidateCommentID(id); err != nil {
		return err
	}
	c, err := client()
	if err != nil {
		return err
	}
	th, err := c.GetCommentThread(slug, id)
	if err != nil {
		return apiErr("getting comment thread", slug, err)
	}

	out := cmd.OutOrStdout()
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		data, err := commentfmt.ThreadAsJSON(slug, *th)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	}
	_, err = fmt.Fprint(out, commentfmt.ThreadMarkdown(slug, *th))
	return err
}
