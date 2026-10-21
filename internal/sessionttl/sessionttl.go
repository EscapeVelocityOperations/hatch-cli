// Package sessionttl parses and formats the per-egg magic-link session
// lifetime (h-pbhn) shared by `hatch protect email` and the MCP tools.
package sessionttl

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Bounds mirror hatch-api's apps.email_session_ttl_seconds CHECK constraint.
const (
	Default = 12 * time.Hour
	Min     = time.Hour
	Max     = 30 * 24 * time.Hour
)

// Parse turns a user-supplied lifetime into the seconds value the API takes:
// "default" → 0 (back to the 12 h default); otherwise a Go duration ("12h",
// "90m") or a whole number of days ("30d"), between 1 h and 30 days, in
// whole seconds.
func Parse(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "default" {
		return 0, nil
	}
	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid session TTL %q: use e.g. 12h, 7d, 30d or default", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(s); err != nil {
			return 0, fmt.Errorf("invalid session TTL %q: use e.g. 12h, 7d, 30d or default", s)
		}
	}
	if d < Min || d > Max {
		return 0, fmt.Errorf("session TTL %q out of range: must be between 1h and 30d", s)
	}
	if d%time.Second != 0 {
		return 0, fmt.Errorf("session TTL %q must be a whole number of seconds", s)
	}
	return int(d / time.Second), nil
}

// Format renders an effective lifetime in seconds as the shortest exact
// form: "30d", "12h", "90m", else Go's duration string.
func Format(seconds int) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case seconds <= 0:
		return "unknown"
	case d%(24*time.Hour) == 0:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	case d%time.Hour == 0:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d%time.Minute == 0:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	default:
		return d.String()
	}
}
