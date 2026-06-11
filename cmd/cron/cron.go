// Package cron implements `hatch cron` — scheduled commands run in an app's
// image via Nomad batch+Periodic (spec h-k6yeh): add, list, rm, logs.
package cron

import (
	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

// APIClient is the interface for the cron endpoints of the Hatch API.
type APIClient interface {
	CreateCron(slug, schedule, command string) (*api.CronJob, error)
	ListCrons(slug string) ([]api.CronJob, error)
	DeleteCron(slug, cronID string) error
	ListCronRuns(slug, cronID string) ([]api.CronRun, error)
	GetCronRunLogs(slug, cronID, runID string) (string, error)
}

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	ResolveSlug  func() (string, error)
	NewAPIClient func(token string) APIClient
}

// stubClient satisfies APIClient until the real client methods exist.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
type stubClient struct{}

func (stubClient) CreateCron(slug, schedule, command string) (*api.CronJob, error) {
	return nil, nil
}
func (stubClient) ListCrons(slug string) ([]api.CronJob, error)        { return nil, nil }
func (stubClient) DeleteCron(slug, cronID string) error                { return nil }
func (stubClient) ListCronRuns(slug, cronID string) ([]api.CronRun, error) {
	return nil, nil
}
func (stubClient) GetCronRunLogs(slug, cronID, runID string) (string, error) {
	return "", nil
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		// STUB(h-p7lvr): real slug resolution (.hatch.toml / git remote) in h-wb661.
		ResolveSlug: func() (string, error) { return "", nil },
		// STUB(h-p7lvr): real api.Client wiring in h-wb661.
		NewAPIClient: func(token string) APIClient { return stubClient{} },
	}
}

var deps = defaultDeps()

var logsRunID string

// NewCmd returns the cron command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage scheduled commands (cron jobs)",
	}

	add := &cobra.Command{
		Use:   "add <schedule> -- <command>",
		Short: "Add a cron job to the app",
		RunE:  runCronAdd,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's cron jobs",
		RunE:  runCronList,
	}
	rm := &cobra.Command{
		Use:   "rm <cron-id>",
		Short: "Remove a cron job",
		RunE:  runCronRm,
	}
	logs := &cobra.Command{
		Use:   "logs <cron-id>",
		Short: "Show logs from a cron job's runs",
		RunE:  runCronLogs,
	}
	logs.Flags().StringVar(&logsRunID, "run", "", "show logs for a specific run (default: latest)")

	cmd.AddCommand(add, list, rm, logs)
	return cmd
}

// runCronAdd handles `hatch cron add <schedule> -- <command>`.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
func runCronAdd(cmd *cobra.Command, args []string) error {
	return nil
}

// runCronList handles `hatch cron list`.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
func runCronList(cmd *cobra.Command, args []string) error {
	return nil
}

// runCronRm handles `hatch cron rm <cron-id>`.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
func runCronRm(cmd *cobra.Command, args []string) error {
	return nil
}

// runCronLogs handles `hatch cron logs <cron-id> [--run <run-id>]`.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
func runCronLogs(cmd *cobra.Command, args []string) error {
	return nil
}
