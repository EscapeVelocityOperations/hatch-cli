// Package stats implements `hatch stats` (alias `hatch metrics`) — one-shot
// per-app metrics table from GET /v1/apps/{slug}/metrics (h-cqajs, spec
// h-ogx7t). `--watch` re-polls every 5s client-side.
//
// Stub for h-e5w0d (tests-first); the impl-cli step replaces the run body
// and wires the API client + root registration.
package stats

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
)

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
	return &Deps{}
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
	return errors.New("not implemented") // TODO(impl-cli)
}
