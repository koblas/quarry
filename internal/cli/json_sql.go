package cli

import (
	"math"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// sqlDocument is sql's --json stdout shape.
type sqlDocument struct {
	Columns   []sqlColumnDocument `json:"columns"`
	Rows      [][]any             `json:"rows"`
	RowCount  int                 `json:"row_count"`
	Limit     int                 `json:"limit"`
	Truncated bool                `json:"truncated"`
	Warnings  []string            `json:"warnings"`
}

// sqlColumnDocument is one entry of "columns": the column's name and its
// DuckDB type name, verbatim.
type sqlColumnDocument struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// renderSQLJSON renders result as sql's --json document. row_count counts
// the rows in the document, limit echoes the --limit value, and columns and
// rows are [] rather than null when empty.
func renderSQLJSON(result report.QueryResult, limit int, warnings []string) ([]byte, error) {
	columns := make([]sqlColumnDocument, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = sqlColumnDocument{Name: col.Name, Type: col.Type}
	}
	rows := make([][]any, len(result.Rows))
	for i, row := range result.Rows {
		cells := make([]any, len(row))
		for j, value := range row {
			cells[j] = jsonSQLCell(result.Columns[j], value)
		}
		rows[i] = cells
	}
	return marshalDocument(sqlDocument{
		Columns: columns, Rows: rows, RowCount: len(rows),
		Limit: limit, Truncated: result.Truncated, Warnings: warnings,
	})
}

// jsonSQLCell is the JSON value of one result cell, the only producer of
// row values: nil, bool, int64, uint64, a finite float32 or float64, or a
// string. NaN and the infinities become "nan", "inf" and "-inf"; a DATE
// prints as its calendar day, any other time as RFC 3339 in UTC; every
// other kind, DECIMAL and HUGEINT included, is DuckDB's own text.
func jsonSQLCell(col store.QueryColumn, v store.QueryValue) any {
	if v.Null {
		return nil
	}
	switch n := v.Native.(type) {
	case bool, int64, uint64:
		return n
	case float64:
		return jsonFloat(n, n)
	case float32:
		return jsonFloat(float64(n), n)
	case time.Time:
		if col.Type == "DATE" {
			return n.Format(jsonDateLayout)
		}
		return n.UTC().Format(time.RFC3339Nano)
	}
	return v.Text
}

// jsonFloat is finite as it is, else the string "nan", "inf" or "-inf";
// finite carries the value in its own width so a FLOAT keeps its short form.
func jsonFloat(f float64, finite any) any {
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
