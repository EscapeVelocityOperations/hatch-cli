package sessionttl

import "testing"

func TestParse(t *testing.T) {
	ok := map[string]int{
		"default":  0,
		" Default": 0,
		"1h":       3600,
		"12h":      43200,
		"90m":      5400,
		"7d":       604800,
		"30d":      2592000,
		"720h":     2592000,
	}
	for in, want := range ok {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "59m", "31d", "721h", "0", "0d", "-1h", "abc", "1.5d", "d", "1h0.5s", "forever"} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %d, want an error", in, got)
		}
	}
}

func TestFormat(t *testing.T) {
	for in, want := range map[int]string{
		43200:   "12h",
		2592000: "30d",
		86400:   "1d",
		5400:    "90m",
		3601:    "1h0m1s",
		0:       "unknown",
	} {
		if got := Format(in); got != want {
			t.Errorf("Format(%d) = %q, want %q", in, got, want)
		}
	}
}
