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
	Type   string `json:"type"`
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
}

// Domain represents a custom domain configuration.
type Domain struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
	CNAME  string `json:"cname,omitempty"`
}
