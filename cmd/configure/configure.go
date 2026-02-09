package configure

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "configure",
		Short: "Configure Hatch CLI with your API token",
		Long:  "Set up your Hatch CLI by providing an API token from https://gethatch.eu/dashboard/tokens",
		RunE:  runConfigure,
	}
}

func runConfigure(cmd *cobra.Command, args []string) error {
	fmt.Println()
	fmt.Println("  Welcome to Hatch CLI configuration!")
	fmt.Println()
	fmt.Println("  Get your API token from: https://gethatch.eu/dashboard/tokens")
	fmt.Println()
	fmt.Print("  API Token: ")

	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	token = strings.TrimSpace(token)

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if !strings.HasPrefix(token, "hatch_") {
		return fmt.Errorf("invalid token format (should start with 'hatch_')")
	}

	// Load existing config to preserve other fields
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	cfg.Token = token

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	ui.Success("Configuration saved to ~/.hatch/config.json")
	fmt.Println()
	fmt.Println("  You're ready to deploy! Run:")
	fmt.Println("    cd your-project")
	fmt.Println("    hatch deploy")
	fmt.Println()

	return nil
}
