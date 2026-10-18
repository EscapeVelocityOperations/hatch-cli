// Package deploywait polls an egg after an artifact upload until the
// asynchronous rollout reaches a terminal status (h-59ej).
//
// The control plane builds the image synchronously, then rolls out the job
// and routes in the background: the upload response only means "deploying".
// The app status is the terminal signal — running on success, failed (with
// status_reason) when the rollout gives up. The deployment row is not used:
// it can stay "deploying" after an async failure.
package deploywait

import (
	"fmt"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

const (
	// DefaultInterval is the delay between two status polls.
	DefaultInterval = 2 * time.Second
	// DefaultTimeout exceeds the control plane's rollout readiness window (~4 min).
	DefaultTimeout = 6 * time.Minute
	// DefaultMaxConsecutiveErrors is the number of failed polls in a row
	// after which waiting is abandoned; fewer are treated as transient.
	DefaultMaxConsecutiveErrors = 3
)

// Options tunes Wait. Zero values select the defaults.
type Options struct {
	Interval             time.Duration
	Timeout              time.Duration
	MaxConsecutiveErrors int
	Sleep                func(time.Duration)
	Now                  func() time.Time
	// OnStatus is called each time the observed status changes.
	OnStatus func(status string)
}

func (o Options) withDefaults() Options {
	if o.Interval <= 0 {
		o.Interval = DefaultInterval
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxConsecutiveErrors <= 0 {
		o.MaxConsecutiveErrors = DefaultMaxConsecutiveErrors
	}
	if o.Sleep == nil {
		o.Sleep = time.Sleep
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// IsSuccess reports whether status means the deploy is live.
func IsSuccess(status string) bool {
	return status == "running" || status == "sleeping"
}

// IsFailure reports whether status means the deploy ended without a live app.
func IsFailure(status string) bool {
	switch status {
	case "failed", "stopped", "crashed":
		return true
	}
	return false
}

// FailedError is returned when the egg reaches a failure status.
type FailedError struct {
	Slug   string
	Status string
	Reason string
}

func (e *FailedError) Error() string {
	msg := fmt.Sprintf("deploy of %s failed: status %s", e.Slug, e.Status)
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// TimeoutError is returned when no terminal status is observed in time.
type TimeoutError struct {
	Slug       string
	LastStatus string
	Timeout    time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timed out after %s waiting for deploy of %s (last status: %s)", e.Timeout, e.Slug, e.LastStatus)
}

// Wait polls get until the egg reaches a terminal status. It returns the last
// app observed (nil if no poll succeeded) and a nil error only on success.
func Wait(slug string, get func() (*api.App, error), opts Options) (*api.App, error) {
	opts = opts.withDefaults()
	deadline := opts.Now().Add(opts.Timeout)

	var last *api.App
	lastStatus := ""
	consecutiveErrors := 0
	for {
		app, err := get()
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= opts.MaxConsecutiveErrors {
				return last, fmt.Errorf("checking deploy status of %s: %w", slug, err)
			}
		} else {
			consecutiveErrors = 0
			last = app
			if app.Status != lastStatus {
				lastStatus = app.Status
				if opts.OnStatus != nil {
					opts.OnStatus(app.Status)
				}
			}
			switch {
			case IsSuccess(app.Status):
				return app, nil
			case IsFailure(app.Status):
				return app, &FailedError{Slug: slug, Status: app.Status, Reason: app.StatusReason}
			}
		}

		if !opts.Now().Before(deadline) {
			status := lastStatus
			if status == "" {
				status = "unknown"
			}
			return last, &TimeoutError{Slug: slug, LastStatus: status, Timeout: opts.Timeout}
		}
		opts.Sleep(opts.Interval)
	}
}
