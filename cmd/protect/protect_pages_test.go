package protect

import (
	"bytes"
	"errors"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

type fakePagesLister struct {
	list *api.PageList
	err  error
}

func (f fakePagesLister) ListPages(string) (*api.PageList, error) { return f.list, f.err }

// h-8iz8 D11: turning protection off warns when the egg's pages stop being
// served, and stays quiet otherwise.
func TestWarnUnservedPages(t *testing.T) {
	two := []api.PageSummary{{Name: "a"}, {Name: "b"}}
	for _, tc := range []struct {
		name   string
		client any
		want   string
	}{
		{"unprotected with pages", fakePagesLister{list: &api.PageList{Pages: two}}, "Note: e has 2 Hatch page(s)"},
		{"still protected", fakePagesLister{list: &api.PageList{Pages: two, Gateway: true}}, ""},
		{"no pages", fakePagesLister{list: &api.PageList{}}, ""},
		{"lookup error", fakePagesLister{err: errors.New("boom")}, ""},
		{"client without pages", struct{}{}, ""},
	} {
		var buf bytes.Buffer
		warnUnservedPages(&buf, tc.client, "e")
		if got := buf.String(); (tc.want == "" && got != "") || (tc.want != "" && !bytes.Contains(buf.Bytes(), []byte(tc.want))) {
			t.Errorf("%s: output %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The production adapters must expose ListPages, or the warning never fires.
func TestRealAdaptersListPages(t *testing.T) {
	var _ pagesLister = &realEmailAPIClient{}
	var _ pagesLister = &realPasswordAPIClient{}
	if _, ok := defaultEmailDeps().NewAPIClient("tok").(pagesLister); !ok {
		t.Error("email adapter does not list pages")
	}
	if _, ok := defaultPasswordDeps().NewAPIClient("tok").(pagesLister); !ok {
		t.Error("password adapter does not list pages")
	}
}
