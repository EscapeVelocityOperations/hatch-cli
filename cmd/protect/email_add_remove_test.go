package protect

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunEmailAdd_MergesAndDedupes (T-304): add on an existing set merges
// (read-modify-write) and dedupes; "@domain" args are domains.
func TestRunEmailAdd_MergesAndDedupes(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, Domains: []string{"corp.com"}}, nil
		},
	}
	withTestEmailDeps(t, mock)

	// "a@b.com" already present (must not duplicate), "c@d.com" is new,
	// "@corp.com" already present as a domain, "@new.com" is a new domain.
	if _, err := captureStdout(func() error {
		return runEmailAdd(&cobra.Command{}, []string{"a@b.com", "c@d.com", "@corp.com", "@new.com"})
	}); err != nil {
		t.Fatalf("runEmailAdd: %v", err)
	}

	if len(mock.lastSetEmails) != 2 || mock.lastSetEmails[0] != "a@b.com" || mock.lastSetEmails[1] != "c@d.com" {
		t.Errorf("emails posted = %v, want [a@b.com c@d.com] (merged + deduped)", mock.lastSetEmails)
	}
	if len(mock.lastSetDomains) != 2 || mock.lastSetDomains[0] != "corp.com" || mock.lastSetDomains[1] != "new.com" {
		t.Errorf("domains posted = %v, want [corp.com new.com] (merged + deduped, @ stripped)", mock.lastSetDomains)
	}
}

// TestRunEmailRemove_UnknownItemErrorsCleanly (T-304): removing an item not
// on the allowlist errors without calling SetEmailProtection.
func TestRunEmailRemove_UnknownItemErrorsCleanly(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}}, nil
		},
	}
	withTestEmailDeps(t, mock)

	err := runEmailRemove(&cobra.Command{}, []string{"nobody@nowhere.com"})
	if err == nil {
		t.Fatal("want an error when removing an item not on the allowlist")
	}
	if !strings.Contains(err.Error(), "nobody@nowhere.com") {
		t.Errorf("error = %v, want it to name the unknown item", err)
	}
	if mock.lastSetEmails != nil {
		t.Error("an unknown item must not reach SetEmailProtection")
	}
}

// TestRunEmailRemove_RemovesKnownItem (T-304): removing a known email
// leaves the rest of the set intact.
func TestRunEmailRemove_RemovesKnownItem(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com", "c@d.com"}, Domains: []string{"corp.com"}}, nil
		},
	}
	withTestEmailDeps(t, mock)

	if err := runEmailRemove(&cobra.Command{}, []string{"a@b.com", "@corp.com"}); err != nil {
		t.Fatalf("runEmailRemove: %v", err)
	}
	if len(mock.lastSetEmails) != 1 || mock.lastSetEmails[0] != "c@d.com" {
		t.Errorf("emails posted = %v, want [c@d.com]", mock.lastSetEmails)
	}
	if len(mock.lastSetDomains) != 0 {
		t.Errorf("domains posted = %v, want []", mock.lastSetDomains)
	}
}
