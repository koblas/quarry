package document

import (
	"math"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// SQL is sql's --json document and the query tool's structured result.
type SQL struct {
	Columns   []SQLColumn `json:"columns"`
	Rows      [][]any     `json:"rows"`
	RowCount  int         `json:"row_count"`
	Limit     int         `json:"limit"`
	Truncated bool        `json:"truncated"`
	Warnings  []string    `json:"warnings"`
}

// SQLColumn is one entry of "columns": the column's name and its DuckDB type name, verbatim.
type SQLColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NewSQL converts result into the sql document: row_count counts its rows, limit echoes the
// row cap asked for, and columns, rows and warnings are empty arrays, never null.
func NewSQL(result report.QueryResult, limit int, warnings []string) SQL {
	columns := make([]SQLColumn, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = SQLColumn{Name: col.Name, Type: col.Type}
	}
	rows := make([][]any, len(result.Rows))
	for i, row := range result.Rows {
		cells := make([]any, len(row))
		for j, value := range row {
			cells[j] = sqlCell(result.Columns[j], value)
		}
		rows[i] = cells
	}
	return SQL{
		Columns: columns, Rows: rows, RowCount: len(rows),
		Limit: limit, Truncated: result.Truncated, Warnings: append([]string{}, warnings...),
	}
}

// sqlCell is one result cell's JSON value, and the only producer of row values:
// nil, bool, int64, uint64, a finite float32 or float64, or a string.
func sqlCell(col store.QueryColumn, v store.QueryValue) any {
	if v.Null {
		return nil
	}
	switch n := v.Native.(type) {
	case bool, int64, uint64:
		return n
	case float64:
		return sqlFloat(n, n)
	case float32:
		return sqlFloat(float64(n), n)
	case time.Time:
		if col.Type == "DATE" {
			return n.Format(DateLayout)
		}
		return n.UTC().Format(time.RFC3339Nano)
	}
	// Every other kind, DECIMAL and HUGEINT included, is DuckDB's own text.
	return v.Text
}

// sqlFloat is finite as it is, else the string "nan", "inf" or "-inf";
// finite carries the value in its own width so a FLOAT keeps its short form.
func sqlFloat(f float64, finite any) any {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return finite
}
