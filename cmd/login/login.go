package login

import (
	"context"
	"fmt"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
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
	GenerateState func() (string, error)
	NewServer     func(port int, state string) auth.Server
	OpenBrowser   func(url string) error
	SaveToken     func(token string) error
}

func defaultDeps() *Deps {
	return &Deps{
		IsLoggedIn:    auth.IsLoggedIn,
		GenerateState: auth.GenerateState,
		NewServer: func(port int, state string) auth.Server {
			return auth.NewCallbackServer(port, state)
		},
		OpenBrowser: auth.OpenBrowser,
		SaveToken:   auth.SaveToken,
	}
}

// deps is package-level for test injection.
var deps = defaultDeps()

func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Hatch via browser",
		RunE:  runLogin,
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

	fmt.Println("Login successful!")
	return nil
}
