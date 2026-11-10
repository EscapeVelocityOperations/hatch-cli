package exitcode

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapNil(t *testing.T) {
	if Wrap(1, nil) != nil {
		t.Fatal("Wrap(1, nil) must stay nil")
	}
}

func TestFrom(t *testing.T) {
	base := errors.New("boom")
	err := fmt.Errorf("context: %w", Wrap(3, base))
	code, ok := From(err)
	if !ok || code != 3 {
		t.Fatalf("From = %d, %v; want 3, true", code, ok)
	}
	if !errors.Is(err, base) {
		t.Fatal("wrapped error must unwrap to the original")
	}
	if err.Error() != "context: boom" {
		t.Fatalf("message = %q", err.Error())
	}
	if _, ok := From(base); ok {
		t.Fatal("plain error must not carry an exit code")
	}
}
