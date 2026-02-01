package auth

import (
	"testing"
)

func TestGenerateStateReturnsHexString(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("state length = %d, want 32", len(state))
	}
}

func TestGenerateStateIsUnique(t *testing.T) {
	s1, _ := GenerateState()
	s2, _ := GenerateState()
	if s1 == s2 {
		t.Error("expected unique states, got identical")
	}
}
