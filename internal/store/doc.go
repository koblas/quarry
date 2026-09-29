// Package store defines quarry's own schema as plain row types: one struct
// per table, plus Rows, Counts and Result, and QueryResult for an ad-hoc
// query's columns and values. It has no database driver
// dependency, so any package can hold quarry's row shapes without linking
// DuckDB. internal/store/duckstore builds these rows into an actual DuckDB
// file.
package store
