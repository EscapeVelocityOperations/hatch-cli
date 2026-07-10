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

// PasswordProtection is the CLI-facing view of an egg's password-protection
// state.
type PasswordProtection struct {
	Protected bool
}

// PasswordAPIClient is the password-protection surface of the Hatch API.
type PasswordAPIClient interface {
	SetPasswordProtection(slug, password string) (*PasswordProtection, error)
	GetPasswordProtection(slug string) (*PasswordProtection, error)
	DeletePasswordProtection(slug string) error
}

// PasswordDeps holds injectable dependencies for testing (cmd/webhook pattern).
type PasswordDeps struct {
	GetToken     func() (string, error)
	GetCwd       func() (string, error)
	NewAPIClient func(token string) PasswordAPIClient
}

var passwordDeps = defaultPasswordDeps()

func defaultPasswordDeps() *PasswordDeps {
	return &PasswordDeps{
		GetToken: auth.GetToken,
		GetCwd:   os.Getwd,
		NewAPIClient: func(token string) PasswordAPIClient {
			return &realPasswordAPIClient{client: api.NewClient(token)}
		},
	}
}

// realPasswordAPIClient adapts *api.Client to the PasswordAPIClient surface.
type realPasswordAPIClient struct{ client *api.Client }

func cliPasswordProtection(pp api.PasswordProtection) PasswordProtection {
	return PasswordProtection{Protected: pp.Protected}
}

func (r *realPasswordAPIClient) SetPasswordProtection(slug, password string) (*PasswordProtection, error) {
	pp, err := r.client.SetPasswordProtection(slug, password)
	if err != nil {
		return nil, err
	}
	c := cliPasswordProtection(*pp)
	return &c, nil
}

func (r *realPasswordAPIClient) GetPasswordProtection(slug string) (*PasswordProtection, error) {
	pp, err := r.client.GetPasswordProtection(slug)
	if err != nil {
		return nil, err
	}
	c := cliPasswordProtection(*pp)
	return &c, nil
}

func (r *realPasswordAPIClient) DeletePasswordProtection(slug string) error {
	return r.client.DeletePasswordProtection(slug)
}

// resolvePasswordApp returns the password-protection API client and the
// current app slug, or a friendly error (not logged in / no app in this
// directory). Mirrors resolveEmailApp.
func resolvePasswordApp() (PasswordAPIClient, string, error) {
	token, err := passwordDeps.GetToken()
	if err != nil || token == "" {
		return nil, "", errors.New("not logged in (run 'hatch login' first)")
	}
	if passwordDeps.NewAPIClient == nil {
		return nil, "", errors.New("protect commands are not yet wired to the API")
	}

	dir := "."
	if passwordDeps.GetCwd != nil {
		d, err := passwordDeps.GetCwd()
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

	return passwordDeps.NewAPIClient(token), slug, nil
}

// runProtect implements `hatch protect` (no verb subcommand — password
// protection is a single credential, not a list, so it takes flags directly
// on the parent command instead of enable/disable/status verbs).
func runProtect(cmd *cobra.Command, args []string) error {
	client, slug, err := resolvePasswordApp()
	if err != nil {
		return err
	}

	password, _ := cmd.Flags().GetString("password")
	if _, err := client.SetPasswordProtection(slug, password); err != nil {
		return fmt.Errorf("enabling password protection: %w", err)
	}

	fmt.Printf("Password protection enabled for %s; enforced by the Hatch auth-gateway.\n", slug)
	return nil
}
