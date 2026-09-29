// Package duckstore builds quarry's DuckDB store from store.Rows: it holds
// the schema DDL, bulk-loads rows through the driver's Appender, and swaps
// the built file in only after a clean checkpoint, so a failed or
// interrupted build never touches the store already in place. It is the
// only package outside internal/platform/duckdb that imports the DuckDB
// driver.
package duckstore
