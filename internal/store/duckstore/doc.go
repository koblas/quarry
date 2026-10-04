// Package duckstore builds quarry's DuckDB store from store.Rows: it holds
// the schema DDL, bulk-loads rows through the driver's Appender, and swaps
// the built file in only after a clean checkpoint, so a failed or
// interrupted build never touches the store already in place. A rebuild
// carries the previous store's import_runs rows and findings into the new
// file, and records the findings the build's detectors report (findings,
// finding_items). A finished store carries one store_info row (FormatVersion,
// quarry's version, build time), appended last, and the v_account_balances,
// v_cash_flow, v_spending and v_holdings views. holding_shares records each
// holding's share count over time, built from the same walk of the investment
// transactions that CheckShares runs; v_holdings values each holding per day
// held, and Holdings reads it for one day. A build asks its RatesSource for the
// exchange rates it needs and stores them in fx_rates; the views carry
// converted columns (balance_cad, amount_usd, spent_cad, usd_cad and their
// siblings) computed from that table. It also answers the read commands: every
// read opens the store read-only, never creating it, and closes it before
// returning; BuiltFrom reports the snapshot the store's latest import run
// recorded.
// It is the only package outside internal/platform/duckdb that imports the
// DuckDB driver.
package duckstore
