// Package stats implements `hatch stats` (alias `hatch metrics`) — one-shot
// per-app metrics table from GET /v1/apps/{slug}/metrics (h-cqajs, spec
// h-ogx7t). `--watch` re-polls every 5s client-side.
//
// Stub for h-e5w0d (tests-first); the impl-cli step replaces the run body
// and wires the API client + root registration.
package stats

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

// watchInterval is the client-side re-poll cadence for `--watch`.
const watchInterval = 5 * time.Second

// AppMetrics mirrors the API's merged metrics payload (v1 fields).
type AppMetrics struct {
	Status        string    `json:"status"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryMB      int       `json:"memory_mb"`
	MemoryLimitMB int       `json:"memory_limit_mb"`
	LastDeployAt  time.Time `json:"last_deploy_at"`
	WakesToday    int       `json:"wakes_today"`
	SampledAt     time.Time `json:"sampled_at"`
}

// Deps are the injectable dependencies (domain/volume cmd pattern).
type Deps struct {
	GetToken   func() (string, error)
	GetMetrics func(token, slug string) (AppMetrics, error)
}

var deps = defaultDeps()

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		GetMetrics: func(token, slug string) (AppMetrics, error) {
			m, err := api.NewClient(token).GetMetrics(slug)
			if err != nil {
				return AppMetrics{}, err
			}
			return AppMetrics(m), nil
		},
	}
}

// NewCmd returns the `hatch stats` command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stats <slug>",
		Aliases: []string{"metrics"},
		Short:   "Show app metrics (CPU, memory, uptime, wakes)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			watch, _ := cmd.Flags().GetBool("watch")
			return runStats(args[0], watch)
		},
	}
	cmd.Flags().Bool("watch", false, "re-poll every 5 seconds")
	return cmd
}

func runStats(slug string, watch bool) error {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return errors.New("not logged in — run `hatch login` first")
	}

	render := func() error {
		m, err := deps.GetMetrics(token, slug)
		if err != nil {
			return err
		}
		printMetricsTable(slug, m)
		return nil
	}

	if !watch {
		return render()
	}
	for {
		if err := render(); err != nil {
			return err
		}
		time.Sleep(watchInterval)
	}
}

// printMetricsTable renders the one-shot per-app metrics table to stdout.
func printMetricsTable(slug string, m AppMetrics) {
	uptime := time.Duration(m.UptimeSeconds) * time.Second
	fmt.Printf("App: %s\n", slug)
	fmt.Printf("  Status        %s\n", m.Status)
	fmt.Printf("  CPU           %.1f%%\n", m.CPUPercent)
	fmt.Printf("  Memory        %d MB / %d MB\n", m.MemoryMB, m.MemoryLimitMB)
	fmt.Printf("  Uptime        %s\n", uptime)
	fmt.Printf("  Wakes today   %d\n", m.WakesToday)
	fmt.Printf("  Last deploy   %s\n", formatTime(m.LastDeployAt))
	fmt.Printf("  Sampled       %s\n", formatTime(m.SampledAt))
}

// formatTime renders a timestamp for the table, or "—" when unset.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 MST")
}
