package invite

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/telemetry"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken           func() (string, error)
	ListPendingInvites func(token string) ([]api.PendingInvite, error)
	AcceptInvite       func(token, inviteToken string) (*api.Collaborator, error)
	DeclineInvite      func(token, inviteToken string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		ListPendingInvites: func(token string) ([]api.PendingInvite, error) {
			return api.NewClient(token).ListPendingInvites()
		},
		AcceptInvite: func(token, inviteToken string) (*api.Collaborator, error) {
			return api.NewClient(token).AcceptInvite(inviteToken)
		},
		DeclineInvite: func(token, inviteToken string) error {
			return api.NewClient(token).DeclineInvite(inviteToken)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the invite command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Manage invitations to collaborate on eggs",
		Long: `List, accept, and decline invitations to collaborate on other people's
eggs (ADR-0022). accept/decline take either the invitation ID shown by
'hatch invite ls' or the token from the accept link in the invitation email.

Accepting grants you full operational access to that egg, including
deploying it and its environment variables/secrets — only accept invitations
from people and eggs you recognize.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newAcceptCmd())
	cmd.AddCommand(newDeclineCmd())

	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List your pending invitations",
		Long: `List pending invitations to collaborate on other people's eggs.

Example:
  hatch invite ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}
}

func newAcceptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "accept <id|token>",
		Short: "Accept an invitation",
		Long: `Accept an invitation to collaborate on an egg.

Accepting grants full operational access to that egg, including deploying it
and its environment variables/secrets. Only accept invitations from people
and eggs you recognize.

Example:
  hatch invite accept 3f2b1c9e-...   # ID from 'hatch invite ls'
  hatch invite accept abc123...       # token from the invitation email`,
		Args:        cobra.ExactArgs(1),
		Annotations: sensitiveArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccept(args[0])
		},
	}
}

func newDeclineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decline <id|token>",
		Short: "Decline an invitation",
		Long: `Decline an invitation to collaborate on an egg.

Example:
  hatch invite decline 3f2b1c9e-...   # ID from 'hatch invite ls'
  hatch invite decline abc123...       # token from the invitation email`,
		Args:        cobra.ExactArgs(1),
		Annotations: sensitiveArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecline(args[0])
		},
	}
}

// sensitiveArgs keeps the positional (possibly an invite token) out of
// telemetry (h-e3s1k.1).
var sensitiveArgs = map[string]string{telemetry.SensitiveArgsAnnotation: "true"}

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

func runList() error {
	token, err := getAuthToken()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Fetching invitations...")
	sp.Start()
	invites, err := deps.ListPendingInvites(token)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching invitations: %w", err)
	}

	if len(invites) == 0 {
		ui.Info("No pending invitations.")
		return nil
	}

	// ID is the invitation row ID, not the secret invite token: accept and
	// decline resolve it to the token themselves (h-e3s1k.2).
	table := ui.NewTable(os.Stdout, "ID", "EGG", "INVITED BY")
	for _, inv := range invites {
		table.AddRow(inv.ID, fmt.Sprintf("%s (%s)", inv.AppName, inv.AppSlug), inv.InvitedByEmail)
	}
	table.Render()
	fmt.Println()
	fmt.Printf("Use the accept link from the invitation email, or run %s\n", ui.Bold("hatch invite accept <id>"))
	return nil
}

// resolveInvite maps ref (invitation ID or invite token) to a pending
// invitation of the authenticated user, rejecting unknown refs instead of
// reporting an action the API silently ignored (h-e3s1k.2).
func resolveInvite(authToken, ref string) (*api.PendingInvite, error) {
	invites, err := deps.ListPendingInvites(authToken)
	if err != nil {
		return nil, fmt.Errorf("fetching invitations: %w", err)
	}
	return api.FindPendingInvite(invites, ref)
}

func runAccept(ref string) error {
	authToken, err := getAuthToken()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Accepting invitation...")
	sp.Start()
	var collab *api.Collaborator
	inv, err := resolveInvite(authToken, ref)
	if err == nil {
		collab, err = deps.AcceptInvite(authToken, inv.InviteToken)
	}
	sp.Stop()

	if err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}

	ui.Success("Invitation accepted — you can now deploy and view logs for this egg.")
	fmt.Printf("  %s Status: %s\n", ui.Dim("→"), ui.Bold(collab.Status))
	return nil
}

func runDecline(ref string) error {
	authToken, err := getAuthToken()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Declining invitation...")
	sp.Start()
	inv, err := resolveInvite(authToken, ref)
	if err == nil {
		err = deps.DeclineInvite(authToken, inv.InviteToken)
	}
	sp.Stop()

	if err != nil {
		return fmt.Errorf("declining invitation: %w", err)
	}

	ui.Success("Invitation declined.")
	return nil
}
