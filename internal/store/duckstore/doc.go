// Package duckstore builds quarry's DuckDB store from store.Rows: it holds
// the schema DDL, bulk-loads rows through the driver's Appender, and swaps
// the built file in only after a clean checkpoint, so a failed or
// interrupted build never touches the store already in place. A finished
// store carries one store_info row (FormatVersion, quarry's version, build
// time), appended last, and the v_account_balances view. It also answers the
// read commands: every read opens the store read-only, never creating it, and
// closes it before returning.
// It is the only package outside internal/platform/duckdb that imports the
// DuckDB driver.
package duckstore
