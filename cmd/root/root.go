package root

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/deploy"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/login"
	"github.com/EscapeVelocityOperations/hatch-cli/cmd/logout"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(deploy.NewCmd())
	rootCmd.AddCommand(login.NewCmd())
	rootCmd.AddCommand(logout.NewCmd())
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
