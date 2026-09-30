package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/store"
)

// sqlColumnGap separates the sql table's columns.
const sqlColumnGap = "  "

// sqlCellEscaper escapes the only characters that would break a table line.
var sqlCellEscaper = strings.NewReplacer("\n", `\n`, "\t", `\t`, "\r", `\r`)

// renderSQLTable renders result as a header of column names, then one line
// per row; numeric columns right-aligned, no line ending in padding.
func renderSQLTable(result store.QueryResult) string {
	numeric := make([]bool, len(result.Columns))
	header := make([]string, len(result.Columns))
	for i, c := range result.Columns {
		numeric[i] = c.Numeric()
		header[i] = sqlCellEscaper.Replace(c.Name)
	}
	lines := make([][]string, 0, 1+len(result.Rows))
	lines = append(lines, header)
	for _, row := range result.Rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = sqlCellEscaper.Replace(v.Text)
		}
		lines = append(lines, cells)
	}

	// Widths are measured after escaping, so an escaped character widens its column.
	widths := make([]int, len(header))
	for _, cells := range lines {
		for i, cell := range cells {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	var b strings.Builder
	for _, cells := range lines {
		b.WriteString(sqlLine(cells, widths, numeric))
		b.WriteString("\n")
	}
	return b.String()
}

// sqlLine lays out one line's cells in their columns, cut after the last
// cell's text so the line ends in no padding.
func sqlLine(cells []string, widths []int, numeric []bool) string {
	var line strings.Builder
	end := 0
	for i, cell := range cells {
		if i > 0 {
			line.WriteString(sqlColumnGap)
		}
		if numeric[i] {
			line.WriteString(padLeft(cell, widths[i]))
		} else {
			line.WriteString(cell)
		}
		if cell != "" {
			end = line.Len()
		}
		if !numeric[i] {
			line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)))
		}
	}
	return line.String()[:end]
}

// renderSQLCSV renders result as CSV: a header line of column names, then one
// line per row, every field written as csvField does.
func renderSQLCSV(result store.QueryResult) string {
	header := make([]csvCell, len(result.Columns))
	for i, c := range result.Columns {
		header[i] = csvCell{Text: c.Name}
	}
	var b strings.Builder
	b.WriteString(csvRecord(header))
	for _, row := range result.Rows {
		cells := make([]csvCell, len(row))
		for i, v := range row {
			cells[i] = csvCell{Text: v.Text, Null: v.Null}
		}
		b.WriteString(csvRecord(cells))
	}
	return b.String()
}
