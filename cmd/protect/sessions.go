package protect

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// SessionsAPIClient is the session-revocation surface of the Hatch API
// (h-pbhn).
type SessionsAPIClient interface {
	RevokeSessions(slug string) error
}

// SessionsDeps holds injectable dependencies for testing (cmd/webhook pattern).
type SessionsDeps struct {
	GetToken        func() (string, error)
	GetCwd          func() (string, error)
	StdinIsTerminal func() bool
	NewAPIClient    func(token string) SessionsAPIClient
}

var sessionsDeps = defaultSessionsDeps()

func defaultSessionsDeps() *SessionsDeps {
	return &SessionsDeps{
		GetToken:        auth.GetToken,
		GetCwd:          os.Getwd,
		StdinIsTerminal: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		NewAPIClient: func(token string) SessionsAPIClient {
			return api.NewClient(token)
		},
	}
}

// newSessionsCmd returns the `hatch protect sessions` group. It has no RunE
// of its own, so cobra only dispatches to its subcommands.
func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage visitor sessions of a protected egg",
	}
	revoke := &cobra.Command{
		Use:   "revoke [slug]",
		Short: "Sign every visitor out (password and magic-link sessions)",
		Long: "Sign every visitor out of a protected egg by rotating its session secret. " +
			"Password and magic-link sessions alike end at once; visitors sign in again. " +
			"The egg is the [slug] argument if given, otherwise the app in the current directory.",
		Args: cobra.MaximumNArgs(1),
		RunE: runSessionsRevoke,
	}
	revoke.Flags().Bool("yes", false, "Skip the confirmation prompt (required when stdin is not a terminal)")
	cmd.AddCommand(revoke)
	return cmd
}

func resolveSessionsApp(args []string) (SessionsAPIClient, string, error) {
	token, err := sessionsDeps.GetToken()
	if err != nil || token == "" {
		return nil, "", errors.New("not logged in (run 'hatch login' first)")
	}
	if len(args) > 0 && args[0] != "" {
		return sessionsDeps.NewAPIClient(token), args[0], nil
	}
	dir := "."
	if sessionsDeps.GetCwd != nil {
		d, err := sessionsDeps.GetCwd()
		if err != nil {
			return nil, "", fmt.Errorf("resolving working directory: %w", err)
		}
		dir = d
	}
	slug := resolve.SlugFromDir(dir)
	if slug == "" {
		return nil, "", errors.New("no app specified — pass a slug " +
			"('hatch protect sessions revoke <slug>') or run from an app directory with a .hatch.toml")
	}
	return sessionsDeps.NewAPIClient(token), slug, nil
}

func runSessionsRevoke(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveSessionsApp(args)
	if err != nil {
		return err
	}

	if yes, _ := cmd.Flags().GetBool("yes"); !yes {
		if !sessionsDeps.StdinIsTerminal() {
			return errors.New("refusing to revoke sessions without --yes: stdin is not a terminal")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "This signs every visitor out of %s; they will have to sign in again.\n", slug)
		fmt.Fprintf(cmd.ErrOrStderr(), "Type %q to confirm: ", slug)
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if strings.TrimSpace(answer) != slug {
			fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled. Egg name did not match.")
			return nil
		}
	}

	if err := client.RevokeSessions(slug); err != nil {
		return fmt.Errorf("revoking sessions: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "All visitor sessions of %s revoked.\n", slug)
	return nil
}
