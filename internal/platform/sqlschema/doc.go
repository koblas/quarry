// Package sqlschema describes a SQLite schema as table and column names,
// fingerprints it for exact-match comparison, and diffs two schemas into
// missing and unexpected tables and columns. It knows nothing about Quicken
// or any other consumer's scoping rules.
package sqlschema
