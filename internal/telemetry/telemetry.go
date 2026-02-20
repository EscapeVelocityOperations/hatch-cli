package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// Version is set by the root package at init time.
var Version = "dev"

// APIHost is the base URL for the telemetry endpoint.
var APIHost = api.DefaultHost

// Event represents a CLI error telemetry event.
type Event struct {
	Command    string `json:"command"`
	Args       string `json:"args"`
	Error      string `json:"error"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	CLIVersion string `json:"cli_version"`
	Mode       string `json:"mode"`
}

// kvRedactRegex matches key=value patterns where value may contain sensitive data.
var kvRedactRegex = regexp.MustCompile(`((?i:token|key|password|secret|credential|auth))\s*[=:]\s*\S+`)

// redact sanitizes error messages by removing tokens and key=value secrets.
func redact(s string) string {
	s = api.RedactToken(s)
	s = kvRedactRegex.ReplaceAllString(s, "${1}=****")
	return s
}

// redactArgs sanitizes command arguments, replacing values of sensitive flags.
func redactArgs(args string) string {
	return redact(args)
}

// Send fires a telemetry event in a background goroutine.
// It is fire-and-forget: errors are silently ignored.
func Send(command, args, errMsg, mode string) {
	if errMsg == "" {
		return
	}

	event := Event{
		Command:    command,
		Args:       redactArgs(args),
		Error:      redact(errMsg),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CLIVersion: Version,
		Mode:       mode,
	}

	go func() {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}

		host := APIHost
		if !strings.HasPrefix(host, "http") {
			host = "https://" + host
		}
		url := host + "/telemetry"

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Post(url, "application/json", bytes.NewReader(data))
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}
