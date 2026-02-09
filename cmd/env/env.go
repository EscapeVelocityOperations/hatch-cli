package env

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	GetEnvVars   func(token, slug string) ([]api.EnvVar, error)
	SetEnvVar    func(token, slug, key, value string) error
	UnsetEnvVar  func(token, slug, key string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) {
			return api.NewClient(token).GetEnvVars(slug)
		},
		SetEnvVar: func(token, slug, key, value string) error {
			return api.NewClient(token).SetEnvVar(slug, key, value)
		},
		UnsetEnvVar: func(token, slug, key string) error {
			return api.NewClient(token).UnsetEnvVar(slug, key)
		},
	}
}

var deps = defaultDeps()

var appSlug string
var envFile string

// NewCmd returns the env command with set/unset subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage environment variables",
		Long:  "List, set, and unset environment variables for a Hatch egg.",
		RunE:  runList,
	}
	cmd.PersistentFlags().StringVarP(&appSlug, "app", "a", "", "egg slug (auto-detected from git remote if omitted)")

	setCmd := &cobra.Command{
		Use:   "set [KEY=VALUE...]",
		Short: "Set environment variables",
		Long:  "Set one or more environment variables on a Hatch egg. Values should be in KEY=VALUE format, or use --from-env to import from a .env file.",
		Args:  cobra.ArbitraryArgs,
		RunE:  runSet,
	}
	setCmd.Flags().StringVarP(&envFile, "from-env", "f", "", "path to .env file to import")

	unsetCmd := &cobra.Command{
		Use:   "unset KEY [KEY...]",
		Short: "Remove environment variables",
		Long:  "Remove one or more environment variables from a Hatch egg.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runUnset,
	}

	cmd.AddCommand(setCmd, unsetCmd)
	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug()
	if err != nil {
		return err
	}

	sp := ui.NewSpinner("Fetching environment variables...")
	sp.Start()
	vars, err := deps.GetEnvVars(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching env vars: %w", err)
	}

	if len(vars) == 0 {
		ui.Info(fmt.Sprintf("No environment variables set for %s.", slug))
		return nil
	}

	table := ui.NewTable(os.Stdout, "KEY", "VALUE")
	for _, v := range vars {
		// Mask sensitive values
		displayValue := v.Value
		sensitiveKeys := []string{"PASSWORD", "SECRET", "TOKEN", "KEY", "DSN", "DATABASE_URL", "API_KEY", "PRIVATE"}
		for _, sk := range sensitiveKeys {
			if strings.Contains(strings.ToUpper(v.Key), sk) {
				if len(v.Value) > 8 {
					displayValue = v.Value[:4] + "****" + v.Value[len(v.Value)-4:]
				} else {
					displayValue = "****"
				}
				break
			}
		}
		table.AddRow(v.Key, displayValue)
	}
	table.Render()
	return nil
}

func runSet(cmd *cobra.Command, args []string) error {
	// Validate inputs
	if envFile == "" && len(args) == 0 {
		return fmt.Errorf("no environment variables specified. Provide KEY=VALUE arguments or use --from-env")
	}

	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug()
	if err != nil {
		return err
	}

	// Process .env file if specified
	if envFile != "" {
		if err := processEnvFile(token, slug, envFile); err != nil {
			return err
		}
	}

	// Process positional KEY=VALUE arguments
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format %q, expected KEY=VALUE", arg)
		}
		key, value := parts[0], parts[1]

		if err := deps.SetEnvVar(token, slug, key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		ui.Success(fmt.Sprintf("Set %s", key))
	}
	return nil
}

// processEnvFile reads a .env file and sets each variable
func processEnvFile(token, slug, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening .env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first =
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format at line %d: %q (expected KEY=VALUE)", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Trim surrounding quotes from value
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if err := deps.SetEnvVar(token, slug, key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		ui.Success(fmt.Sprintf("Set %s", key))
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading .env file: %w", err)
	}

	return nil
}

func runUnset(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug()
	if err != nil {
		return err
	}

	for _, key := range args {
		if err := deps.UnsetEnvVar(token, slug, key); err != nil {
			return fmt.Errorf("unsetting %s: %w", key, err)
		}
		ui.Success(fmt.Sprintf("Unset %s", key))
	}
	return nil
}

func resolveSlug() (string, error) {
	if appSlug != "" {
		// Try to resolve as app name by listing apps
		token, err := deps.GetToken()
		if err == nil && token != "" {
			client := api.NewClient(token)
			apps, err := client.ListApps()
			if err == nil {
				// Check if appSlug matches any app name or slug
				for _, app := range apps {
					if app.Name == appSlug || app.Slug == appSlug {
						return app.Slug, nil
					}
				}
			}
		}
		// Fall back to using it directly as slug
		return appSlug, nil
	}
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no egg specified and no hatch git remote found. Use --app <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}
