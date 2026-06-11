package api

import "time"

// App represents a deployed application on Hatch.
type App struct {
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	StatusReason string    `json:"status_reason,omitempty"`
	URL          string    `json:"url"`
	Region       string    `json:"region"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AppResources is the per-app resource override echoed by
// PATCH /v1/apps/{slug}/resources (h-ek431). A nil field means "no override".
type AppResources struct {
	Slug     string `json:"slug"`
	MemoryMB *int   `json:"memory_mb"`
	CPUMHz   *int   `json:"cpu_mhz"`
}

// AppMetrics is the merged per-app metrics payload from
// GET /v1/apps/{slug}/metrics (h-cqajs).
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

// Deployment represents a deployment record.
type Deployment struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Commit    string    `json:"commit"`
	CreatedAt time.Time `json:"created_at"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LogEntry represents a single log line from SSE streaming.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

// Addon represents a provisioned addon (database, storage, etc).
type Addon struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	URL                string `json:"url,omitempty"`
	DatabaseURL        string `json:"database_url,omitempty"`
	PostgresBytesUsed  *int64 `json:"postgres_bytes_used,omitempty"`
	PostgresRowsUsed   *int64 `json:"postgres_rows_used,omitempty"`
	PostgresLimitBytes *int64 `json:"postgres_limit_bytes,omitempty"`
	PostgresLimitRows  *int64 `json:"postgres_limit_rows,omitempty"`
	WritesBlocked      *bool  `json:"postgres_writes_blocked,omitempty"`
}

// Domain represents a custom domain configuration.
type Domain struct {
	Domain            string `json:"domain"`
	Status            string `json:"status"`
	CNAME             string `json:"cname,omitempty"`
	Verified          bool   `json:"verified"`
	VerificationToken string `json:"verification_token,omitempty"`
}

// APIKey represents an API key for the user.
type APIKey struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Prefix     string    `json:"prefix"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

// BoostCredit represents a single boost credit.
type BoostCredit struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	GrantedAt string `json:"granted_at"`
}

// BoostCreditsResponse represents the response from GET /v1/boost-credits.
type BoostCreditsResponse struct {
	DayCredits  int64         `json:"day_credits"`
	WeekCredits int64         `json:"week_credits"`
	Credits     []BoostCredit `json:"credits"`
}

// RedeemCreditResponse represents the response from POST /v1/boost-credits/{id}/redeem.
type RedeemCreditResponse struct {
	CreditID       string `json:"credit_id"`
	EggSlug        string `json:"egg_slug"`
	Type           string `json:"type"`
	BoostExpiresAt string `json:"boost_expires_at"`
}

// CronJob represents a scheduled command on an app. The list endpoint
// enriches each cron with its last run and the computed next run.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
type CronJob struct {
	ID            string     `json:"id"`
	Schedule      string     `json:"schedule"`
	Command       string     `json:"command"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"created_at"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
}

// CronRun represents one execution of a cron job.
// STUB(h-p7lvr): implemented in h-wb661 (impl-cli).
type CronRun struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"` // running|success|failed|skipped_depleted
	ExitCode   int       `json:"exit_code"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// Preview is a PR preview environment of a parent app (h-qtie8).
// Wire shape matches the API's preview responses.
type Preview struct {
	Slug      string    `json:"slug"`
	PRNumber  int       `json:"pr_number"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	Created   bool      `json:"created"`
}
