package ci

import (
	"errors"
	"strings"
	"testing"
)

const tokenFixture = "hatch_0123456789abcdef0123456789abcdef"

// h-db2jl B1: the token is extracted STRICTLY — a line that is exactly a
// hatch_… token. Anything else on the stream (messages, prompts) is ignored,
// and a stream without exactly one token is refused.
func TestExtractToken(t *testing.T) {
	human := "Created deploy token \"ci-acme\"\n\n  " + tokenFixture + "\n  Save this now — store it as the HATCH_TOKEN secret in your CI; it cannot be retrieved later.\n"
	for name, tc := range map[string]struct {
		in      string
		want    string
		wantErr error
	}{
		"raw token with newline":       {in: tokenFixture + "\n", want: tokenFixture},
		"raw token without newline":    {in: tokenFixture, want: tokenFixture},
		"surrounding whitespace":       {in: "  " + tokenFixture + " \r\n", want: tokenFixture},
		"legacy human keys create":     {in: human, want: tokenFixture},
		"same token twice is one":      {in: tokenFixture + "\n" + tokenFixture + "\n", want: tokenFixture},
		"empty input":                  {in: "", wantErr: ErrNoToken},
		"blank lines only":             {in: "\n  \n", wantErr: ErrNoToken},
		"message line only (B1)":       {in: "Created deploy token \"ci-acme\"\n", wantErr: ErrNoToken},
		"not a hatch token":            {in: "ghp_notahatchtoken\n", wantErr: ErrNoToken},
		"token embedded in a sentence": {in: "your token is " + tokenFixture + "\n", wantErr: ErrNoToken},
		"prefix only":                  {in: "hatch_\n", wantErr: ErrNoToken},
		"two different tokens":         {in: tokenFixture + "\nhatch_ffffffffffffffff\n", wantErr: ErrAmbiguousToken},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ExtractToken(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("a refused input must yield no token, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A refused stream never echoes its content: it may be some other secret.
func TestExtractToken_ErrorsNeverEchoInput(t *testing.T) {
	const secretish = "sk-live-this-is-some-other-secret"
	for _, in := range []string{secretish + "\n", tokenFixture + "\nhatch_ffffffffffffffff\n"} {
		_, err := ExtractToken(in)
		if err == nil {
			t.Fatalf("expected an error for %q", in)
		}
		for _, leak := range []string{secretish, tokenFixture, "hatch_ffffffffffffffff"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("error %q leaks input %q", err, leak)
			}
		}
	}
}

// ReadTokenFrom reads the whole (bounded) stream, then extracts; an empty
// stream is not an error (the caller reports "nothing on stdin").
func TestReadTokenFrom(t *testing.T) {
	got, err := ReadTokenFrom(strings.NewReader("Created deploy token \"x\"\n\n  " + tokenFixture + "\n"))
	if err != nil || got != tokenFixture {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := ReadTokenFrom(strings.NewReader("")); err != nil || got != "" {
		t.Fatalf("empty stream: got %q, %v; want \"\", nil", got, err)
	}
	if _, err := ReadTokenFrom(strings.NewReader("hello\n")); !errors.Is(err, ErrNoToken) {
		t.Fatalf("non-token stream: err = %v, want ErrNoToken", err)
	}
}
