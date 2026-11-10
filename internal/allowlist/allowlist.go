// Package allowlist provides list operations shared by hatch-cli's
// email-protection MCP tools.
package allowlist

// RemoveAll returns existing with every entry in remove filtered out,
// preserving order. Comparison is exact-string; normalization is the
// caller's job. An entry in remove that isn't present in existing is a
// no-op, not an error — looser than the CLI's `protect email remove`,
// which errors on a typo (h-7b9l/h-dmd4 T-002).
func RemoveAll(existing, remove []string) []string {
	skip := make(map[string]struct{}, len(remove))
	for _, r := range remove {
		skip[r] = struct{}{}
	}
	out := make([]string, 0, len(existing))
	for _, e := range existing {
		if _, ok := skip[e]; !ok {
			out = append(out, e)
		}
	}
	return out
}
