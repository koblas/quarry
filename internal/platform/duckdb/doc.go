// Package duckdb wraps github.com/duckdb/duckdb-go/v2 behind quarry's own
// DB type: exclusive-create, checkpoint-before-close, and bulk row append
// through the driver's Appender. Money enters and leaves this package only
// as duckdb.Decimal or a CAST(... AS VARCHAR) string, never float64.
//
// This package (and the store adapter that consumes it) is the only place
// in quarry that imports the driver, so other test binaries never link it.
package duckdb
