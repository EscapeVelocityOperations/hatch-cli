package ui

import (
	"fmt"
	"io"
	"strings"
)

type Table struct {
	headers []string
	rows    [][]string
	out     io.Writer
}

func NewTable(out io.Writer, headers ...string) *Table {
	return &Table{
		headers: headers,
		out:     out,
	}
}

func (t *Table) AddRow(cols ...string) {
	t.rows = append(t.rows, cols)
}

func (t *Table) Render() {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = len(h)
	}
	for _, row := range t.rows {
		for i, col := range row {
			if i < len(widths) && len(col) > widths[i] {
				widths[i] = len(col)
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
				fmt.Fprintf(t.out, "%-*s", widths[i]+2, col)
			}
		}
		fmt.Fprintln(t.out)
	}
}
