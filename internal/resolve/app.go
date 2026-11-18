package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// AppSlug prefers exact slugs over names. Unknown values remain literal slugs.
// Ambiguous names require the caller to choose an explicit slug.
func AppSlug(value string, apps []api.App) (string, error) {
	for _, app := range apps {
		if app.Slug == value {
			return app.Slug, nil
		}
	}
	var matches []string
	for _, app := range apps {
		if app.Name == value {
			matches = append(matches, app.Slug)
		}
	}
	switch len(matches) {
	case 0:
		return value, nil
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("ambiguous egg name %q; use an explicit slug: %s", value, strings.Join(matches, ", "))
	}
}
