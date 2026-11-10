package mcpserver

import "testing"

// h-e2hf/h-gveh D7: MCP tool results feed agent context/telemetry, so any
// account identity surfaced there must be masked, never the full address.
func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"typical address", "eric@voxist.com", "e***@voxist.com"},
		{"one-char local part", "a@b.com", "a***@b.com"},
		{"empty string", "", ""},
		{"no at sign", "notanemail", ""},
		{"at sign with no local part", "@nodomain.com", ""},
		{"at sign with no domain", "user@", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskEmail(tt.email); got != tt.want {
				t.Errorf("maskEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}
