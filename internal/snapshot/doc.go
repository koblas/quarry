// Package snapshot takes a verified, private copy of an open Quicken bundle:
// it opens the live database read-only, backs it up with SQLite's online
// backup API, checks the copy's integrity and schema against a caller-
// supplied reference, and writes both the snapshot and its JSON manifest
// under a snapshots directory. SyncAndImport then sequences a store build
// through a consumer-declared Importer, skipping it when the schema check
// found a mismatch; ImportFrom does the same from an earlier snapshot,
// re-verifying its hash and schema without reading Quicken. List reads the
// snapshots directory newest first and marks the snapshot the store was built from;
// Prune deletes all but the newest few, never that one, and a Server built
// WithAutoPrune does the same after each successful store build. Sync's store
// build and Prune each run under quarry's lock file: LockForSync and
// LockForPrune take it, so only one writer runs at a time.
package snapshot
