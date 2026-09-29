package duckdb

import "context"

// Table is a query result: its columns and, in order, its rows.
type Table struct {
	Columns []Column
	Rows    [][]Value
}

// Column is one result column: its name and its DuckDB type name, e.g.
// DECIMAL(18,2) or STRUCT("a" INTEGER).
type Column struct {
	Name string
	Type string
}

// Value is one result cell. Text is DuckDB's own text for it (what
// CAST(v AS VARCHAR) gives; "NULL" for a NULL). Native is nil, a bool,
// int64, uint64, float32 (FLOAT) or float64 (DOUBLE), a time.Time for a
// finite DATE or TIMESTAMP, or Text for every other type, DECIMAL included.
type Value struct {
	Null   bool
	Text   string
	Native any
}

// UnprintableValueError is QueryTable's refusal of a column holding a value
// it cannot render as DuckDB's text, such as a JSON value the driver has
// already decoded.
type UnprintableValueError struct {
	Column string
	Type   string
}

func (e *UnprintableValueError) Error() string {
	return `cannot print column "` + e.Column + `" of type ` + e.Type
}

// QueryTable runs query verbatim and returns its result, stopping after
// maxRows rows (every row when maxRows is 0 or less). A query or driver
// fault is returned as the driver's own error, unwrapped; a value it cannot
// render is an *UnprintableValueError.
func (d *DB) QueryTable(ctx context.Context, query string, maxRows int) (Table, error) {
	rows, err := d.conn.QueryContext(ctx, query)
	if err != nil {
		return Table{}, err //nolint:wrapcheck // callers classify the driver's own error
	}
	defer func() { _ = rows.Close() }()

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		// unreachable: database/sql's Rows.ColumnTypes errs only once rs.closed, and QueryContext just returned rows open.
		return Table{}, err //nolint:wrapcheck // callers classify the driver's own error
	}
	table := Table{Columns: make([]Column, len(columnTypes)), Rows: [][]Value{}}
	types := make([]*typeNode, len(columnTypes))
	for i, ct := range columnTypes {
		table.Columns[i] = Column{Name: ct.Name(), Type: ct.DatabaseTypeName()}
		types[i], _ = parseTypeName(table.Columns[i].Type)
	}

	raw := make([]any, len(columnTypes))
	dest := make([]any, len(columnTypes))
	for i := range raw {
		dest[i] = &raw[i]
	}
	for (maxRows <= 0 || len(table.Rows) < maxRows) && rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			// unreachable: rows is open, dest has one *any per column, and database/sql's convertAssign stores any driver value in an *any.
			return Table{}, err //nolint:wrapcheck // callers classify the driver's own error
		}
		row := make([]Value, len(raw))
		for i, v := range raw {
			value, ok := cellValue(types[i], v)
			if !ok {
				return Table{}, &UnprintableValueError{Column: table.Columns[i].Name, Type: table.Columns[i].Type}
			}
			row[i] = value
		}
		table.Rows = append(table.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Table{}, err //nolint:wrapcheck // callers classify the driver's own error
	}
	return table, nil
}
