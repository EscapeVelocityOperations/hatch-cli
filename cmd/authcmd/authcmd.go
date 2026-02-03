package authcmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	callbackPort = 8765
	authTimeout  = 5 * time.Minute
	authBaseURL  = "https://gethatch.eu/cli-auth"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	IsLoggedIn    func() (bool, error)
	GetToken      func() (string, error)
	GenerateState func() (string, error)
	NewServer     func(port int, state string) auth.Server
	OpenBrowser   func(url string) error
	SaveToken     func(token string) error
	ClearToken    func() error
	ListKeys      func(token string) ([]api.APIKey, error)
	GetTokenSource func() string
}

func defaultDeps() *Deps {
	return &Deps{
		IsLoggedIn:    auth.IsLoggedIn,
		GetToken:      auth.GetToken,
		GenerateState: auth.GenerateState,
		NewServer: func(port int, state string) auth.Server {
			return auth.NewCallbackServer(port, state)
		},
		OpenBrowser: auth.OpenBrowser,
		SaveToken:   auth.SaveToken,
		ClearToken:  auth.ClearToken,
		ListKeys: func(token string) ([]api.APIKey, error) {
			return api.NewClient(token).ListKeys()
		},
		GetTokenSource: getTokenSource,
	}
}

var deps = defaultDeps()

// NewCmd returns the auth command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long:  "Login, logout, check status, and manage API keys for your Hatch account.",
	}

	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newKeysCmd())

	return cmd
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Hatch via browser",
		Long:  "Opens a browser window for OAuth authentication and stores the API key.",
		RunE:  runLogin,
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of Hatch",
		Long:  "Removes stored credentials from the config file.",
		RunE:  runLogout,
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long:  "Display current authentication state and token source.",
		RunE:  runStatus,
	}
}

func newKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "List your API keys",
		Long:  "List all API keys associated with your account.",
		RunE:  runKeys,
	}
}

func runLogin(cmd *cobra.Command, args []string) error {
	loggedIn, err := deps.IsLoggedIn()
	if err != nil {
		return fmt.Errorf("checking auth status: %w", err)
	}
	if loggedIn {
		fmt.Println("Already logged in.")
		return nil
	}

	state, err := deps.GenerateState()
	if err != nil {
		return fmt.Errorf("generating state: %w", err)
	}

	srv := deps.NewServer(callbackPort, state)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer srv.Close()

	authURL := fmt.Sprintf("%s?state=%s&port=%d", authBaseURL, state, callbackPort)
	fmt.Println("Opening browser for authentication...")
	if err := deps.OpenBrowser(authURL); err != nil {
		fmt.Printf("Could not open browser. Please visit:\n  %s\n", authURL)
	}

	fmt.Println("Waiting for authentication...")

	ctx, cancel := context.WithTimeout(cmd.Context(), authTimeout)
	defer cancel()

	token, err := srv.WaitForResult(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := deps.SaveToken(token); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	ui.Success("Login successful!")
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	if err := deps.ClearToken(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	ui.Success("Logged out successfully.")
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	loggedIn, err := deps.IsLoggedIn()
	if err != nil {
		return fmt.Errorf("checking auth status: %w", err)
	}

	fmt.Println()
	if loggedIn {
		source := deps.GetTokenSource()
		ui.Success("Logged in")
		fmt.Printf("  %s Token source: %s\n", ui.Dim("→"), ui.Bold(source))
	} else {
		ui.Warn("Not logged in")
		fmt.Printf("  %s Run 'hatch auth login' to authenticate\n", ui.Dim("→"))
	}
	fmt.Println()

	return nil
}

func runKeys(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch auth login', set HATCH_TOKEN, or use --token")
	}

	sp := ui.NewSpinner("Fetching API keys...")
	sp.Start()
	keys, err := deps.ListKeys(token)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching keys: %w", err)
	}

	if len(keys) == 0 {
		ui.Info("No API keys found.")
		return nil
	}

	table := ui.NewTable(os.Stdout, "NAME", "PREFIX", "CREATED", "LAST USED")
	for _, k := range keys {
		lastUsed := "Never"
		if !k.LastUsedAt.IsZero() {
			lastUsed = k.LastUsedAt.Format("2006-01-02")
		}
		table.AddRow(
			k.Name,
			k.Prefix+"...",
			k.CreatedAt.Format("2006-01-02"),
			lastUsed,
		)
	}
	table.Render()
	return nil
}

// getTokenSource returns a human-readable description of where the token comes from.
func getTokenSource() string {
	if os.Getenv("HATCH_TOKEN") != "" {
		return "HATCH_TOKEN environment variable"
	}
	// Check if --token flag was used by checking if token exists but env doesn't
	// This is a simplification; the actual source check happens during GetToken
	return "config file (~/.hatch/config.json)"
}
