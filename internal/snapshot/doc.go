// Package snapshot takes a verified, private copy of an open Quicken bundle:
// it opens the live database read-only, backs it up with SQLite's online
// backup API, checks the copy's integrity and schema against a caller-
// supplied reference, and writes both the snapshot and its JSON manifest
// under a snapshots directory.
package snapshot
