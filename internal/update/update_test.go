package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"v1.0.1", "v1.0.0", 1},
		{"0.10.3", "0.10.2", 1},
		{"0.10.2", "0.10.3", -1},
		{"0.11.0", "0.10.9", 1},
		{"1.0.0", "0.99.99", 1},
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) || (tt.want == 0 && got != 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFormatNotification(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		if got := FormatNotification(nil); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("no update", func(t *testing.T) {
		r := &CheckResult{UpdateAvailable: false}
		if got := FormatNotification(r); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("update available", func(t *testing.T) {
		r := &CheckResult{
			LatestVersion:   "0.10.3",
			CurrentVersion:  "0.10.2",
			UpdateAvailable: true,
		}
		got := FormatNotification(r)
		if got == "" {
			t.Error("expected non-empty notification")
		}
		if !contains(got, "0.10.2") || !contains(got, "0.10.3") {
			t.Errorf("notification missing versions: %q", got)
		}
	})
}

func TestCheckDevVersion(t *testing.T) {
	result := Check("dev")
	if result != nil {
		t.Error("expected nil for dev version")
	}
}

func TestCheckEmptyVersion(t *testing.T) {
	result := Check("")
	if result != nil {
		t.Error("expected nil for empty version")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
