package logout

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of Hatch",
		RunE:  runLogout,
	}
}

func runLogout(cmd *cobra.Command, args []string) error {
	if err := auth.ClearToken(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	fmt.Println("Logged out successfully.")
	return nil
}
