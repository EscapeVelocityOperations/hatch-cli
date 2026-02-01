package auth

import (
	"runtime"
	"testing"
)

func TestOpenBrowserDoesNotPanic(t *testing.T) {
	// We can't actually open a browser in CI, but we can verify
	// the function doesn't panic for the current platform
	err := OpenBrowser("http://localhost:9999/test")
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		if err == nil {
			t.Error("expected error for unsupported platform")
		}
	}
	// On supported platforms, Start() may return nil even if browser doesn't exist
	// as exec.Cmd.Start() launches async
}
