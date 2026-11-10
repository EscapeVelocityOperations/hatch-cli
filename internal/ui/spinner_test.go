package ui

import (
	"io"
	"os"
	"testing"
)

// Outside a TTY each status change is its own line and the message keeps a
// single ellipsis (h-59ej N4).
func TestSpinnerNonTTYUpdate(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	sp := NewSpinner("Waiting for deploy...")
	sp.Start()
	sp.Update("Waiting for deploy (building)...")
	sp.Update("Waiting for deploy (building)...")
	sp.Update("Waiting for deploy (running)")
	sp.Stop()
	os.Stderr = orig
	_ = w.Close()

	out, _ := io.ReadAll(r)
	want := "Waiting for deploy...\nWaiting for deploy (building)...\nWaiting for deploy (running)... done.\n"
	if string(out) != want {
		t.Fatalf("got %q\nwant %q", out, want)
	}
}
