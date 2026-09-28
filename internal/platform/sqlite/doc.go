// Package sqlite wraps database/sql with the mattn/go-sqlite3 driver for
// quarry's two SQLite needs: a strictly read-only connection to a live file,
// and a private copy taken with SQLite's online backup API. Every schema
// read goes through pragma_table_info so it reflects what SQLite itself
// reports, not what a caller assumes a table looks like.
package sqlite
