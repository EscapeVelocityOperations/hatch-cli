package protect

import (
	"errors"
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/spf13/cobra"
)

// EmailProtection is the CLI-facing view of an egg's email-allowlist state.
type EmailProtection struct {
	Enabled bool
	Emails  []string
	Domains []string
}

// EmailAPIClient is the email-protection surface of the Hatch API.
type EmailAPIClient interface {
	SetEmailProtection(slug string, emails, domains []string) (*EmailProtection, error)
	GetEmailProtection(slug string) (*EmailProtection, error)
	DeleteEmailProtection(slug string) error
}

// EmailDeps holds injectable dependencies for testing (cmd/webhook pattern).
type EmailDeps struct {
	GetToken     func() (string, error)
	GetCwd       func() (string, error)
	NewAPIClient func(token string) EmailAPIClient
}

var emailDeps = defaultEmailDeps()

func defaultEmailDeps() *EmailDeps {
	return &EmailDeps{
		GetToken: auth.GetToken,
		GetCwd:   os.Getwd,
		NewAPIClient: func(token string) EmailAPIClient {
			return &realEmailAPIClient{client: api.NewClient(token)}
		},
	}
}

// realEmailAPIClient adapts *api.Client to the EmailAPIClient surface.
type realEmailAPIClient struct{ client *api.Client }

func cliEmailProtection(ep api.EmailProtection) EmailProtection {
	return EmailProtection{Enabled: ep.EmailProtected, Emails: ep.Emails, Domains: ep.Domains}
}

func (r *realEmailAPIClient) SetEmailProtection(slug string, emails, domains []string) (*EmailProtection, error) {
	ep, err := r.client.SetEmailProtection(slug, emails, domains)
	if err != nil {
		return nil, err
	}
	c := cliEmailProtection(*ep)
	return &c, nil
}

func (r *realEmailAPIClient) GetEmailProtection(slug string) (*EmailProtection, error) {
	ep, err := r.client.GetEmailProtection(slug)
	if err != nil {
		return nil, err
	}
	c := cliEmailProtection(*ep)
	return &c, nil
}

func (r *realEmailAPIClient) DeleteEmailProtection(slug string) error {
	return r.client.DeleteEmailProtection(slug)
}

// NewEmailCmd returns the `hatch protect email` command group.
func NewEmailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Manage email-allowlist protection for this app",
	}
	emailEnableCmd.Flags().StringSlice("email", nil, "Exact email address(es) to allow")
	emailEnableCmd.Flags().StringSlice("domain", nil, "Email domain(s) to allow (with or without a leading @)")
	cmd.AddCommand(emailEnableCmd, emailDisableCmd, emailListCmd, emailAddCmd, emailRemoveCmd)
	return cmd
}

var emailEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable email-allowlist protection (replaces the current lists)",
	RunE:  runEmailEnable,
}

var emailDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable email-allowlist protection",
	RunE:  runEmailDisable,
}

var emailListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the current email allowlist",
	RunE:  runEmailList,
}

var emailAddCmd = &cobra.Command{
	Use:   "add <email-or-@domain>...",
	Short: "Add email(s) or @domain(s) to the allowlist",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmailAdd,
}

var emailRemoveCmd = &cobra.Command{
	Use:   "remove <email-or-@domain>...",
	Short: "Remove email(s) or @domain(s) from the allowlist",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmailRemove,
}

// resolveEmailApp returns the email-protection API client and the current
// app slug, or a friendly error (not logged in / no app in this directory).
func resolveEmailApp() (EmailAPIClient, string, error) {
	token, err := emailDeps.GetToken()
	if err != nil || token == "" {
		return nil, "", errors.New("not logged in (run 'hatch login' first)")
	}
	if emailDeps.NewAPIClient == nil {
		return nil, "", errors.New("protect email commands are not yet wired to the API")
	}

	dir := "."
	if emailDeps.GetCwd != nil {
		d, err := emailDeps.GetCwd()
		if err != nil {
			return nil, "", fmt.Errorf("resolving working directory: %w", err)
		}
		dir = d
	}
	slug := resolve.SlugFromDir(dir)
	if slug == "" {
		return nil, "", errors.New("no app found here — run from an app directory " +
			"with a .hatch.toml (or 'hatch init' first)")
	}

	return emailDeps.NewAPIClient(token), slug, nil
}

// T-302 skeleton: run bodies are placeholders until T-303/T-304 wire the
// real logic (mirrors cmd/webhook's tests-first -> impl-cli history).
func runEmailEnable(cmd *cobra.Command, args []string) error {
	return errors.New("not yet implemented")
}

func runEmailDisable(cmd *cobra.Command, args []string) error {
	return errors.New("not yet implemented")
}

func runEmailList(cmd *cobra.Command, args []string) error {
	return errors.New("not yet implemented")
}

func runEmailAdd(cmd *cobra.Command, args []string) error {
	return errors.New("not yet implemented")
}

func runEmailRemove(cmd *cobra.Command, args []string) error {
	return errors.New("not yet implemented")
}
