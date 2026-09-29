# ADR-002: `quarry sync` sequences the import inside `snapshot.Server`

## Status

Accepted

## Context

Phase 1 makes `quarry sync` take a snapshot and then build quarry's store from it. Two
business rules join the two steps: the import is skipped when the snapshot's schema does
not match the reference, and a failed stdout write reports a different refusal once the
build was reached (O1 before it, O1b after it). Rules like these must not sit in `cli`'s
RunE, which only parses input and formats output. A feature package may not import
another feature package, so `snapshot` cannot call `importer` directly, and `Manifest` /
`MismatchError` must not cross into the importer, which knows only Quicken's schema.

## Decision

`snapshot.Server` owns the sequence. `snapshot` declares a one-method `Importer` port,
`Import(ctx, store.SnapshotRef) (store.Result, error)`, shaped to `(*importer.Server).Import`
so the importer satisfies it with no adapter. `snapshot.WithImporter` injects it. A second
port, `StoreProbe` (`Path`, `Exists`), injected with `snapshot.WithStoreProbe`, locates the
store for refusal copy (the directory for S1–S3, the file for S4/V1/I2) and for the unbuilt
result's path and NOT BUILT / NOT REBUILT line; `cmd/quarry` passes the same
`duckstore.Store` it gives the importer, so only `duckstore` knows where the store lives. A new method, `(*Server).SyncAndImport(ctx, bundlePath)
(Outcome, error)`, calls `Sync`, and imports the committed snapshot only when its schema
verified; `Outcome{Manifest, Store *store.Result}` carries `Store == nil` when no build
was reached. `Sync` keeps its signature and behaviour. The O1/O1b choice is a method on
`Outcome`. An import failure never goes through `FailureOutcome` (the snapshot is already
committed and kept); it is framed as a store refusal that wraps the importer's error, so
`errors.As` still reaches `*importer.UnmappableError`. `cli.ServerFactory` keeps its shape:
`cli` calls `SyncAndImport` and renders the `Outcome`; `cmd/quarry` wires
importer → duckstore.

## Consequences

- **Positive:** Skip-on-mismatch and O1/O1b are tested at the `snapshot.Server` boundary with a fake `Importer`, without linking DuckDB.
- **Positive:** `snapshot`, `cli` and `importer` stay free of the DuckDB driver; only `cmd/quarry` links it (checked with `go list -deps`).
- **Positive:** `--from` (SCENARIO-03) adds a second entry point that reuses the same import step and `Outcome`, with no new wiring in `cli`.
- **Negative:** `snapshot` now orchestrates the whole sync, not only snapshot-taking; its name undersells it, and the store refusal copy (S1–S4, V1, I2) accumulates there.
- **Trade-off:** `snapshot` asks the store where it is and whether it exists, but never opens it. The probe and the importer's store are two ports on one `duckstore.Store` value; only the `cmd/quarry` wiring keeps them the same instance, and its tests name the store path the importer writes.
