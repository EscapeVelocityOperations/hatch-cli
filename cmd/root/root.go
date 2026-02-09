package root

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/apps"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/authcmd"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/boost"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/configure"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/db"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/deploy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/destroy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/domain"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/energy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/env"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/initcmd"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/login"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/logout"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/logs"
	mcpcmd "github.com/EscapeVelocityOperations/hatch-cli/cmd/mcp"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/open"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/restart"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	verbose  bool
	tokenFlag string
)

var rootCmd = &cobra.Command{
	Use:   "hatch",
	Short: "Hatch CLI - Developer tools for Hatch",
	Long:  "Hatch is a command-line interface for deploying and managing eggs on the Hatch platform.",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.hatch/config.json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&tokenFlag, "token", "", "API token (overrides HATCH_TOKEN and config file)")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if tokenFlag != "" {
			auth.SetTokenFlag(tokenFlag)
		}

		// Skip TOS check for commands that don't need it
		name := cmd.Name()
		if name == "version" || name == "help" || name == "login" || name == "mcp" || name == "configure" || name == "init" {
			return
		}

		// Check if TOS already accepted
		cfg, err := config.Load()
		if err != nil {
			return // Don't block on config errors
		}
		if cfg.TosAcceptedAt != "" {
			return // Already accepted
		}

		// Show TOS summary and prompt for acceptance
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════╗")
		fmt.Fprintln(os.Stderr, "║          Hatch — Terms of Service                    ║")
		fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════╝")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Key points:")
		fmt.Fprintln(os.Stderr, "  • Hatch is EXPERIMENTAL — no SLA, no uptime guarantee")
		fmt.Fprintln(os.Stderr, "  • Your data is YOUR responsibility — back up externally")
		fmt.Fprintln(os.Stderr, "  • No health data, banking data, or sensitive personal data")
		fmt.Fprintln(os.Stderr, "  • No crypto mining, spam, malware, or illegal content")
		fmt.Fprintln(os.Stderr, "  • Free plan: apps sleep after 10 min, 2h/day energy")
		fmt.Fprintln(os.Stderr, "  • Accounts violating terms may be suspended")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Full terms: https://gethatch.eu/terms")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprint(os.Stderr, "  Do you accept these terms? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "\n  Terms not accepted. You must accept the terms to use Hatch.")
			os.Exit(1)
		}

		// Save acceptance
		cfg.TosAcceptedAt = time.Now().UTC().Format(time.RFC3339)
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not save TOS acceptance: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "\n  Terms accepted. Welcome to Hatch!")
		}
		fmt.Fprintln(os.Stderr, "")
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(apps.NewCmd())
	rootCmd.AddCommand(apps.NewInfoCmd())
	rootCmd.AddCommand(authcmd.NewCmd())
	rootCmd.AddCommand(boost.NewCmd())
	rootCmd.AddCommand(configure.NewCmd())
	rootCmd.AddCommand(db.NewCmd())
	rootCmd.AddCommand(deploy.NewCmd())
	rootCmd.AddCommand(destroy.NewCmd())
	rootCmd.AddCommand(domain.NewCmd())
	rootCmd.AddCommand(energy.NewCmd())
	rootCmd.AddCommand(initcmd.NewCmd())
	rootCmd.AddCommand(env.NewCmd())
	rootCmd.AddCommand(login.NewCmd())
	rootCmd.AddCommand(logout.NewCmd())
	rootCmd.AddCommand(logs.NewCmd())
	rootCmd.AddCommand(mcpcmd.NewCmd())
	rootCmd.AddCommand(open.NewCmd())
	rootCmd.AddCommand(restart.NewCmd())
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configDir, err := config.Dir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error finding config directory:", err)
			return
		}

		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv()
	viper.ReadInConfig()
}
