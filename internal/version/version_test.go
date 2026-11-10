package version

import "testing"

func TestVersion_DefaultsToDev(t *testing.T) {
	if got := Version(); got != "dev" {
		t.Errorf("Version() = %q, want %q", got, "dev")
	}
}

func TestVersion_Settable(t *testing.T) {
	orig := version
	defer func() { version = orig }()

	version = "1.2.3"
	if got := Version(); got != "1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "1.2.3")
	}
}
