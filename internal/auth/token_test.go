package auth

import (
	"testing"
)

func setupHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
}

func TestIsLoggedInFalseWhenNoToken(t *testing.T) {
	setupHome(t)
	ok, err := IsLoggedIn()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected not logged in")
	}
}

func TestSaveAndGetToken(t *testing.T) {
	setupHome(t)
	if err := SaveToken("abc-123"); err != nil {
		t.Fatal(err)
	}
	tok, err := GetToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "abc-123" {
		t.Errorf("GetToken() = %q, want %q", tok, "abc-123")
	}
}

func TestIsLoggedInTrueAfterSave(t *testing.T) {
	setupHome(t)
	SaveToken("token")
	ok, err := IsLoggedIn()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected logged in")
	}
}

func TestClearTokenMakesLoggedOut(t *testing.T) {
	setupHome(t)
	SaveToken("token")
	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	ok, _ := IsLoggedIn()
	if ok {
		t.Error("expected not logged in after clear")
	}
}
