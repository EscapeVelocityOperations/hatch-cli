package ci

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// TokenPrefix is the prefix of every Hatch deploy token.
const TokenPrefix = "hatch_"

// maxTokenInput bounds how much of a piped stream is read: a deploy token is a
// few dozen bytes, so anything near this is not a token stream.
const maxTokenInput = 64 << 10

// tokenLine matches a line that is exactly a deploy token: the hatch_ prefix
// followed by [A-Za-z0-9_] (the server mints "hatch_" + hex).
var tokenLine = regexp.MustCompile(`^hatch_[A-Za-z0-9_]+$`)

var (
	// ErrNoToken means the input held no deploy token line.
	ErrNoToken = errors.New("no hatch_… deploy token found on stdin (mint one with: hatch auth keys create)")
	// ErrAmbiguousToken means the input held several different deploy tokens.
	ErrAmbiguousToken = errors.New("several different hatch_… deploy tokens found on stdin; pipe exactly one")
)

// ExtractToken returns the deploy token held by input, strictly: a token is a
// line that, once trimmed, is exactly hatch_<chars>. Other lines (the messages
// around a token, prompts) are ignored; input without a token line, or with
// several different ones, is refused. The errors never include the input, which
// may be some other secret.
func ExtractToken(input string) (string, error) {
	var token string
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if !tokenLine.MatchString(line) {
			continue
		}
		if token != "" && token != line {
			return "", ErrAmbiguousToken
		}
		token = line
	}
	if token == "" {
		return "", ErrNoToken
	}
	return token, nil
}

// ReadTokenFrom reads a (bounded) stream and extracts the deploy token from it.
// A stream with nothing but whitespace yields "", nil so the caller can report
// the missing value; any other stream must hold exactly one token.
func ReadTokenFrom(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxTokenInput))
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", nil
	}
	return ExtractToken(string(b))
}
