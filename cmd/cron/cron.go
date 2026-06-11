// Package cron implements `hatch cron` — scheduled commands run in an app's
// image via Nomad batch+Periodic (spec h-k6yeh): add, list, rm, logs.
package cron

import (
	"fmt"
	"strings"
	"time"

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
// STUB(h-wb661): real api.Client cron methods are a follow-up wiring task
// (red tests first) — until then the command group is not registered in the
// root command, so this default is unreachable in production.
type stubClient struct{}

func (stubClient) CreateCron(slug, schedule, command string) (*api.CronJob, error) {
	return nil, fmt.Errorf("cron API client not wired yet")
}
func (stubClient) ListCrons(slug string) ([]api.CronJob, error) {
	return nil, fmt.Errorf("cron API client not wired yet")
}
func (stubClient) DeleteCron(slug, cronID string) error {
	return fmt.Errorf("cron API client not wired yet")
}
func (stubClient) ListCronRuns(slug, cronID string) ([]api.CronRun, error) {
	return nil, fmt.Errorf("cron API client not wired yet")
}
func (stubClient) GetCronRunLogs(slug, cronID, runID string) (string, error) {
	return "", fmt.Errorf("cron API client not wired yet")
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		// STUB(h-wb661): real slug resolution (.hatch.toml / git remote) lands
		// with the wiring follow-up.
		ResolveSlug: func() (string, error) { return "", fmt.Errorf("no app detected") },
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

// requireAuth returns the token or the canonical not-logged-in error.
func requireAuth() (string, error) {
	token, err := deps.GetToken()
	if err != nil {
		return "", fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}
	return token, nil
}

// runCronAdd handles `hatch cron add <schedule> -- <command>`.
func runCronAdd(cmd *cobra.Command, args []string) error {
	token, err := requireAuth()
	if err != nil {
		return err
	}

	// Exactly one arg (the schedule) before the mandatory `--`; everything
	// after it is the user command, joined verbatim.
	dash := cmd.ArgsLenAtDash()
	if dash != 1 || len(args) < 2 {
		return fmt.Errorf("usage: hatch cron add \"<schedule>\" -- <command>")
	}
	schedule := args[0]
	command := strings.Join(args[1:], " ")

	slug, err := deps.ResolveSlug()
	if err != nil {
		return err
	}

	job, err := deps.NewAPIClient(token).CreateCron(slug, schedule, command)
	if err != nil {
		return fmt.Errorf("creating cron: %w", err)
	}
	if job != nil {
		fmt.Printf("Created cron %s on %s: %s → %s\n", job.ID, slug, job.Schedule, job.Command)
	}
	return nil
}

// runCronList handles `hatch cron list`.
func runCronList(cmd *cobra.Command, args []string) error {
	token, err := requireAuth()
	if err != nil {
		return err
	}
	slug, err := deps.ResolveSlug()
	if err != nil {
		return err
	}

	crons, err := deps.NewAPIClient(token).ListCrons(slug)
	if err != nil {
		return fmt.Errorf("listing crons: %w", err)
	}
	if len(crons) == 0 {
		fmt.Printf("No cron jobs on %s.\n", slug)
		return nil
	}

	fmt.Printf("%-10s  %-16s  %-30s  %-9s  %-22s  %-22s\n",
		"ID", "SCHEDULE", "COMMAND", "STATE", "LAST RUN", "NEXT RUN")
	for _, c := range crons {
		state := "enabled"
		if !c.Enabled {
			state = "disabled"
		}
		lastRun := "-"
		if c.LastRunStatus != "" {
			lastRun = c.LastRunStatus
			if c.LastRunAt != nil {
				lastRun += " " + formatCronTime(*c.LastRunAt)
			}
		}
		nextRun := "-"
		if c.NextRunAt != nil {
			nextRun = formatCronTime(*c.NextRunAt)
		}
		fmt.Printf("%-10s  %-16s  %-30s  %-9s  %-22s  %-22s\n",
			c.ID, c.Schedule, c.Command, state, lastRun, nextRun)
	}
	return nil
}

// runCronRm handles `hatch cron rm <cron-id>`.
func runCronRm(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hatch cron rm <cron-id>")
	}
	token, err := requireAuth()
	if err != nil {
		return err
	}
	slug, err := deps.ResolveSlug()
	if err != nil {
		return err
	}

	if err := deps.NewAPIClient(token).DeleteCron(slug, args[0]); err != nil {
		return fmt.Errorf("removing cron %s: %w", args[0], err)
	}
	fmt.Printf("Removed cron %s from %s.\n", args[0], slug)
	return nil
}

// runCronLogs handles `hatch cron logs <cron-id> [--run <run-id>]`. Without
// --run it shows the latest run, resolved client-side from the newest-first
// runs listing.
func runCronLogs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hatch cron logs <cron-id> [--run <run-id>]")
	}
	token, err := requireAuth()
	if err != nil {
		return err
	}
	slug, err := deps.ResolveSlug()
	if err != nil {
		return err
	}
	client := deps.NewAPIClient(token)
	cronID := args[0]

	runID := logsRunID
	if runID == "" {
		runs, err := client.ListCronRuns(slug, cronID)
		if err != nil {
			return fmt.Errorf("listing runs for cron %s: %w", cronID, err)
		}
		if len(runs) == 0 {
			return fmt.Errorf("no runs yet for cron %s", cronID)
		}
		runID = runs[0].ID // newest first
	}

	logs, err := client.GetCronRunLogs(slug, cronID, runID)
	if err != nil {
		return fmt.Errorf("fetching logs for run %s: %w", runID, err)
	}
	fmt.Println(logs)
	return nil
}

// formatCronTime renders run timestamps compactly in UTC.
func formatCronTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
