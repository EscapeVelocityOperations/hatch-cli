package mcpserver

import "strings"

// maskEmail returns a privacy-safe identity string for MCP tool results and
// telemetry (h-y2g6 PII posture): first character + domain, e.g.
// "eric@voxist.com" -> "e***@voxist.com". Invalid/empty input returns "" so
// callers can skip the identity line entirely.
func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return ""
	}
	return email[:1] + "***" + email[at:]
}
