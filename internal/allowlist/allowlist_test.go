package allowlist

import "testing"

// TestRemoveAll (h-7b9l/h-dmd4 T-002): the merge-tools MCP `remove_…` tool
// filters the current allowlist through this, so it must be order-preserving
// (the CLI's remove/add already rely on that ordering being stable) and
// treat an unknown entry as a no-op rather than an error — the MCP remove
// tool no-ops silently on entries not present (unlike the CLI's remove,
// which errors on a typo).
func TestRemoveAll(t *testing.T) {
	cases := []struct {
		name             string
		existing, remove []string
		want             []string
	}{
		{
			name:     "removes present entries, preserves order",
			existing: []string{"a@b.com", "c@d.com", "e@f.com"},
			remove:   []string{"c@d.com"},
			want:     []string{"a@b.com", "e@f.com"},
		},
		{
			name:     "unknown entry is a no-op, not an error",
			existing: []string{"a@b.com"},
			remove:   []string{"zzz@unknown.com"},
			want:     []string{"a@b.com"},
		},
		{
			name:     "removing everything leaves an empty slice",
			existing: []string{"a@b.com"},
			remove:   []string{"a@b.com"},
			want:     []string{},
		},
		{
			name:     "comparison is exact-string — normalization is the caller's job",
			existing: []string{"a@b.com"},
			remove:   []string{"A@B.COM"},
			want:     []string{"a@b.com"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RemoveAll(tc.existing, tc.remove)
			if !stringSlicesEqual(got, tc.want) {
				t.Errorf("RemoveAll(%v, %v) = %v, want %v", tc.existing, tc.remove, got, tc.want)
			}
		})
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
