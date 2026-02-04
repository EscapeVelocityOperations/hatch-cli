package root

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/apps"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/authcmd"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/db"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/deploy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/destroy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/domain"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/env"
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
	Long:  "Hatch is a command-line interface for deploying and managing applications on the Hatch platform.",
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
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(apps.NewCmd())
	rootCmd.AddCommand(apps.NewInfoCmd())
	rootCmd.AddCommand(authcmd.NewCmd())
	rootCmd.AddCommand(db.NewCmd())
	rootCmd.AddCommand(deploy.NewCmd())
	rootCmd.AddCommand(destroy.NewCmd())
	rootCmd.AddCommand(domain.NewCmd())
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
