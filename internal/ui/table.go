package ui

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Table represents a text table for output.
type Table struct {
	headers []string
	rows    [][]string
	out     io.Writer
}

// NewTable creates a new table with the given headers.
func NewTable(out io.Writer, headers ...string) *Table {
	return &Table{
		headers: headers,
		out:     out,
	}
}

// AddRow adds a row to the table.
func (t *Table) AddRow(cols ...string) {
	t.rows = append(t.rows, cols)
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\x1b' || c == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if c >= 'A' && c <= '~' { // End of ANSI sequence
				inEscape = false
			}
			continue
		}
		result.WriteByte(c)
	}
	return result.String()
}

// displayWidth returns the visible width of a string, excluding ANSI escape sequences.
func displayWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

// Render renders the table to the output writer.
func (t *Table) Render() {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = displayWidth(h)
	}
	for _, row := range t.rows {
		for i, col := range row {
			if i < len(widths) {
				w := displayWidth(col)
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	// Header
	for i, h := range t.headers {
		fmt.Fprintf(t.out, "%-*s", widths[i]+2, h)
	}
	fmt.Fprintln(t.out)

	// Separator
	for i := range t.headers {
		fmt.Fprint(t.out, strings.Repeat("─", widths[i]+2))
	}
	fmt.Fprintln(t.out)

	// Rows
	for _, row := range t.rows {
		for i, col := range row {
			if i < len(widths) {
				// Pad with spaces to account for ANSI codes in the content
				padding := widths[i] + 2 - displayWidth(col)
				fmt.Fprint(t.out, col)
				if padding > 0 {
					fmt.Fprint(t.out, strings.Repeat(" ", padding))
				}
			}
		}
		fmt.Fprintln(t.out)
	}
}
