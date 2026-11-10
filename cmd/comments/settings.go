package comments

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/spf13/cobra"
)

func newEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <egg>",
		Short: "Enable comments on an email-protected egg",
		Long: "Enable comments: every HTML page of the egg gets the comment widget for visitors " +
			"signed in by magic link. The egg must be under email protection.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettings(cmd, args[0], "enabling comments", func(c APIClient, slug string) (*api.CommentSettings, error) {
				return c.EnableComments(slug)
			})
		},
	}
}

func newDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <egg>",
		Short: "Disable comments (hide the widget, close writes; comments are kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettings(cmd, args[0], "disabling comments", func(c APIClient, slug string) (*api.CommentSettings, error) {
				return c.DisableComments(slug)
			})
		},
	}
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <egg>",
		Short: "Show whether comments are enabled and how many are stored",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettings(cmd, args[0], "getting comment status", func(c APIClient, slug string) (*api.CommentSettings, error) {
				return c.GetCommentSettings(slug)
			})
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func runSettings(cmd *cobra.Command, slug, action string, call func(APIClient, string) (*api.CommentSettings, error)) error {
	c, err := client()
	if err != nil {
		return err
	}
	s, err := call(c, slug)
	if err != nil {
		return apiErr(action, slug, err)
	}
	out := cmd.OutOrStdout()
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return writeJSON(out, s)
	}
	printSettings(out, slug, s)
	return nil
}

func printSettings(w io.Writer, slug string, s *api.CommentSettings) {
	state := "disabled"
	if s.CommentsEnabled {
		state = "enabled"
	}
	fmt.Fprintf(w, "Comments are %s on %s (%d stored).\n", state, slug, s.CommentCount)
	if !s.EmailProtected {
		fmt.Fprintln(w, "This egg is not under email protection: comments cannot be enabled until it is.")
	}
}

func writeJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
