package store

import "regexp"

// QueryResult is the result of one query against the store.
type QueryResult struct {
	Columns []QueryColumn
	Rows    [][]QueryValue
}

// QueryColumn is one result column: its name and its DuckDB type name.
type QueryColumn struct {
	Name string
	Type string
}

// numericType matches every DuckDB integer, floating-point and DECIMAL type name.
var numericType = regexp.MustCompile(`^(TINYINT|SMALLINT|INTEGER|BIGINT|HUGEINT|BIGNUM|` +
	`UTINYINT|USMALLINT|UINTEGER|UBIGINT|UHUGEINT|FLOAT|DOUBLE|DECIMAL\(\d+,\d+\))$`)

// Numeric reports whether the column holds numbers: an integer, FLOAT,
// DOUBLE or DECIMAL type, not a LIST, ARRAY or STRUCT of them.
func (c QueryColumn) Numeric() bool {
	return numericType.MatchString(c.Type)
}

// QueryValue is one result cell. Text is DuckDB's own text for it ("NULL"
// for a NULL). Native is nil, a bool, int64, uint64, float32 (FLOAT),
// float64 (DOUBLE), a time.Time for a finite DATE or TIMESTAMP, or Text.
type QueryValue struct {
	Null   bool
	Text   string
	Native any
}

// UnprintableValueError is a query refused because a column holds a value
// quarry cannot print as DuckDB's text.
type UnprintableValueError struct {
	Column string
	Type   string
}

func (e *UnprintableValueError) Error() string {
	return `cannot print column "` + e.Column + `" of type ` + e.Type
}
