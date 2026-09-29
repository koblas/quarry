// Package duckdb wraps github.com/duckdb/duckdb-go/v2 behind quarry's own
// DB type: exclusive-create, checkpoint-before-close, bulk row append
// through the driver's Appender, and QueryTable, which returns any query's
// result as DuckDB's own text for each value plus a typed form. Money enters
// and leaves this package only as duckdb.Decimal or text, never float64.
//
// This package (and the store adapter that consumes it) is the only place
// in quarry that imports the driver, so other test binaries never link it.
package duckdb
