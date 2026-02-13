package api

import "time"

// App represents a deployed application on Hatch.
type App struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	URL       string    `json:"url"`
	Region    string    `json:"region"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Type              string `json:"type"`
	Status            string `json:"status"`
	URL               string `json:"url,omitempty"`
	DatabaseURL       string `json:"database_url,omitempty"`
	PostgresBytesUsed *int64 `json:"postgres_bytes_used,omitempty"`
	PostgresRowsUsed  *int64 `json:"postgres_rows_used,omitempty"`
	PostgresLimitBytes *int64 `json:"postgres_limit_bytes,omitempty"`
	PostgresLimitRows  *int64 `json:"postgres_limit_rows,omitempty"`
	WritesBlocked     *bool  `json:"postgres_writes_blocked,omitempty"`
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
