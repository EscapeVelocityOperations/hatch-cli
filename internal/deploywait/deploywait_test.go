package deploywait

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// fakeClock advances only when Sleep is called, so tests never wait.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time        { return c.now }
func (c *fakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }
func (c *fakeClock) opts(timeout time.Duration) Options {
	return Options{Timeout: timeout, Sleep: c.Sleep, Now: c.Now}
}

type step struct {
	status, reason string
	err            error
}

// sequence returns a getter replaying steps; the last one repeats forever.
func sequence(steps ...step) (func() (*api.App, error), *int) {
	calls := 0
	return func() (*api.App, error) {
		s := steps[len(steps)-1]
		if calls < len(steps) {
			s = steps[calls]
		}
		calls++
		if s.err != nil {
			return nil, s.err
		}
		return &api.App{Slug: "egg-a1b2", Status: s.status, StatusReason: s.reason}, nil
	}, &calls
}

func TestWait_BuildingThenRunning(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	get, calls := sequence(step{status: "building"}, step{status: "building"}, step{status: "running"})
	var seen []string
	opts := clk.opts(time.Minute)
	opts.OnStatus = func(s string) { seen = append(seen, s) }

	app, err := Wait("egg-a1b2", get, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Status != "running" || *calls != 3 {
		t.Fatalf("got status %q after %d polls, want running after 3", app.Status, *calls)
	}
	if strings.Join(seen, ",") != "building,running" {
		t.Fatalf("OnStatus saw %v, want only status changes", seen)
	}
}

func TestWait_SleepingIsSuccess(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	get, _ := sequence(step{status: "sleeping"})
	if _, err := Wait("egg-a1b2", get, clk.opts(time.Minute)); err != nil {
		t.Fatalf("sleeping should be success, got %v", err)
	}
}

func TestWait_FailedCarriesReason(t *testing.T) {
	for _, status := range []string{"failed", "stopped", "crashed"} {
		clk := &fakeClock{now: time.Unix(0, 0)}
		get, _ := sequence(step{status: "building"}, step{status: status, reason: "Deploy failed: app did not start in time"})
		app, err := Wait("egg-a1b2", get, clk.opts(time.Minute))
		var fe *FailedError
		if !errors.As(err, &fe) {
			t.Fatalf("%s: want *FailedError, got %v", status, err)
		}
		if app == nil || app.Status != status || !strings.Contains(err.Error(), "app did not start in time") {
			t.Fatalf("%s: got app=%+v err=%v", status, app, err)
		}
	}
}

func TestWait_TimeoutMentionsLastStatus(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	get, calls := sequence(step{status: "building"})
	_, err := Wait("egg-a1b2", get, clk.opts(10*time.Second))
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("want *TimeoutError, got %v", err)
	}
	if te.LastStatus != "building" || !strings.Contains(err.Error(), "building") {
		t.Fatalf("timeout error should name last status, got %v", err)
	}
	// 2s interval over 10s: polls at t=0,2,4,6,8,10.
	if *calls != 6 {
		t.Fatalf("got %d polls, want 6", *calls)
	}
}

func TestWait_UnknownStatusKeepsWaiting(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	get, _ := sequence(step{status: "deploying"}, step{status: "something-new"}, step{status: "running"})
	if _, err := Wait("egg-a1b2", get, clk.opts(time.Minute)); err != nil {
		t.Fatalf("non-terminal statuses must not end the wait, got %v", err)
	}
}

func TestWait_TransientErrorsTolerated(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	netErr := errors.New("connection reset")
	get, _ := sequence(step{err: netErr}, step{err: netErr}, step{status: "building"}, step{err: netErr}, step{err: netErr}, step{status: "running"})
	if _, err := Wait("egg-a1b2", get, clk.opts(time.Minute)); err != nil {
		t.Fatalf("1-2 consecutive errors must be tolerated, got %v", err)
	}
}

func TestWait_PersistentErrorsAbort(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	netErr := errors.New("connection refused")
	get, calls := sequence(step{status: "building"}, step{err: netErr})
	app, err := Wait("egg-a1b2", get, clk.opts(time.Minute))
	if !errors.Is(err, netErr) {
		t.Fatalf("want wrapped network error, got %v", err)
	}
	if *calls != 4 || app == nil || app.Status != "building" {
		t.Fatalf("got %d polls, last app %+v; want abort on 3rd consecutive error with last app kept", *calls, app)
	}
}
