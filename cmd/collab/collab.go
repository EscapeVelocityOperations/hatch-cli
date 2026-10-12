package collab

import (
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken           func() (string, error)
	InviteCollaborator func(token, slug, email string) (*api.Collaborator, error)
	ListCollaborators  func(token, slug string) ([]api.Collaborator, error)
	RemoveCollaborator func(token, slug, collaboratorID string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		InviteCollaborator: func(token, slug, email string) (*api.Collaborator, error) {
			return api.NewClient(token).InviteCollaborator(slug, email)
		},
		ListCollaborators: func(token, slug string) ([]api.Collaborator, error) {
			return api.NewClient(token).ListCollaborators(slug)
		},
		RemoveCollaborator: func(token, slug, collaboratorID string) error {
			return api.NewClient(token).RemoveCollaborator(slug, collaboratorID)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the collab command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collab",
		Short: "Manage collaborators on your eggs",
		Long: `Invite, list, and remove collaborators on a Hatch egg (ADR-0022).

An accepted collaborator can deploy, view logs, and view deployments on the
shared egg. Sharing an egg shares its secrets: a collaborator can deploy,
and deploying exposes the egg's environment variables. Only invite people
you trust with full operational access to this egg.`,
	}

	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newRemoveCmd())

	return cmd
}

func newAddCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "add <email>",
		Short: "Invite a collaborator",
		Long: `Invite someone to collaborate on an egg.

Sharing an egg shares its secrets: an accepted collaborator can deploy the
egg, and deploying exposes its environment variables. Only invite people you
trust with full operational access.

Example:
  hatch collab add friend@example.com --app my-app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(appSlug, args[0])
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

func newListCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List collaborators",
		Long: `List all collaborators for an egg.

Example:
  hatch collab ls --app my-app`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(appSlug)
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

func newRemoveCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "rm <email|id>",
		Short: "Remove a collaborator",
		Long: `Remove a collaborator from an egg, by email or collaborator ID.

Example:
  hatch collab rm friend@example.com --app my-app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(appSlug, args[0])
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

func getAuthToken() (string, error) {
	token, err := deps.GetToken()
	if err != nil {
		return "", fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}
	return token, nil
}

func runAdd(appSlug, email string) error {
	token, err := getAuthToken()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Inviting collaborator...")
	sp.Start()
	collab, err := deps.InviteCollaborator(token, appSlug, email)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("inviting collaborator: %w", err)
	}

	fmt.Println()
	ui.Success(fmt.Sprintf("Invited %s to '%s'", collab.Email, appSlug))
	fmt.Println()
	ui.Warn(fmt.Sprintf("This grants %s full operational access to '%s' once accepted,", collab.Email, appSlug))
	fmt.Println("  including deploying it and viewing its environment variables/secrets.")
	fmt.Println()
	fmt.Printf("  %s They'll receive an email with a link to accept.\n", ui.Dim("→"))
	fmt.Printf("  %s Check status: %s\n", ui.Dim("→"), ui.Bold(fmt.Sprintf("hatch collab ls --app %s", appSlug)))
	fmt.Println()

	return nil
}

func runList(appSlug string) error {
	token, err := getAuthToken()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Fetching collaborators...")
	sp.Start()
	collabs, err := deps.ListCollaborators(token, appSlug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching collaborators: %w", err)
	}

	if len(collabs) == 0 {
		ui.Info(fmt.Sprintf("No collaborators on '%s'.", appSlug))
		return nil
	}

	table := ui.NewTable(os.Stdout, "ID", "EMAIL", "ROLE", "STATUS")
	for _, c := range collabs {
		table.AddRow(c.ID, c.Email, c.Role, statusColor(c.Status))
	}
	table.Render()
	return nil
}

func runRemove(appSlug, target string) error {
	token, err := getAuthToken()
	if err != nil {
		return err
	}

	collaboratorID, err := resolveCollaboratorID(token, appSlug, target)
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Removing collaborator...")
	sp.Start()
	err = deps.RemoveCollaborator(token, appSlug, collaboratorID)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("removing collaborator: %w", err)
	}

	ui.Success(fmt.Sprintf("Removed collaborator from '%s'", appSlug))
	return nil
}

// resolveCollaboratorID accepts either a raw collaborator ID or an email
// address (the server's DELETE endpoint only takes an ID; there is no
// remove-by-email endpoint, so this resolves it client-side).
func resolveCollaboratorID(token, appSlug, target string) (string, error) {
	if !strings.Contains(target, "@") {
		return target, nil
	}

	collabs, err := deps.ListCollaborators(token, appSlug)
	if err != nil {
		return "", fmt.Errorf("fetching collaborators: %w", err)
	}
	for _, c := range collabs {
		if strings.EqualFold(c.Email, target) {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("no collaborator found with email %q on '%s'", target, appSlug)
}

func statusColor(status string) string {
	switch status {
	case "accepted":
		return ui.Green(status)
	case "pending":
		return ui.Yellow(status)
	case "declined":
		return ui.Red(status)
	default:
		return status
	}
}
