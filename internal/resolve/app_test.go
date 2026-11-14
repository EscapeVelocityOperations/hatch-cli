package resolve

import (
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

func TestAppSlug(t *testing.T) {
	tests := []struct {
		name    string
		apps    []api.App
		want    string
		wantErr bool
	}{
		{"slug beats earlier name", []api.App{{Name: "target", Slug: "duplicate"}, {Name: "original", Slug: "target"}}, "target", false},
		{"slug beats later name", []api.App{{Name: "original", Slug: "target"}, {Name: "target", Slug: "duplicate"}}, "target", false},
		{"slug beats ambiguous names", []api.App{{Name: "target", Slug: "b"}, {Name: "target", Slug: "a"}, {Slug: "target"}}, "target", false},
		{"unique name", []api.App{{Name: "target", Slug: "actual"}}, "actual", false},
		{"ambiguous name", []api.App{{Name: "target", Slug: "b"}, {Name: "target", Slug: "a"}}, "", true},
		{"unknown", []api.App{{Name: "other", Slug: "other-slug"}}, "target", false},
		{"empty list", nil, "target", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AppSlug("target", tt.apps)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("AppSlug() = %q, %v; want %q, error=%v", got, err, tt.want, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "use an explicit slug: a, b") {
				t.Fatalf("missing slug choices: %v", err)
			}
		})
	}
}
