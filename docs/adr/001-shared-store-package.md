# ADR-001: quarry's store is a shared lower package, split from its DuckDB adapter

## Status

Accepted

## Context

Phase 1 builds quarry's own DuckDB store from a Quicken snapshot. The importer writes it, the
sync command reports its row counts, and later phases (views, findings, reporting) read it.
A feature package may not import another feature package, so the store cannot live inside
the importer: every later reader would have to import a feature. The store's row types are
also needed by packages that must not link the DuckDB driver — a large cgo dependency whose
link step has run out of memory on small Linux CI runners — so `snapshot`, `cli` and the
importer's own tests must be able to use the types without pulling in the driver.

## Decision

`internal/store` is a shared lower package, beside `internal/platform/*` in the dependency
order: any feature may import it, and it imports no feature. It holds the values features
exchange about the store — quarry's row types (`Rows` and one struct per table, `ImportRun`
included), `SnapshotRef`, `Counts`, `Result` with its `Validation` and `NotImported`, and the
four store sentinels (`ErrValidationFailed`, `ErrStoreNotWritable`, `ErrDiskFull`,
`ErrUnmappable`) — and no behaviour and no driver import.
`internal/store/duckstore` holds the DuckDB schema DDL, the builder, and the atomic swap
(build into a per-run `.quarry-<UTC>-<pid>.duckdb.partial`, checkpoint and close, rename over
`quarry.duckdb`). It is the only package outside `internal/platform/duckdb` that imports the
driver. Only the importer knows Quicken's schema; neither store package knows it. The
importer declares its own `Store` port with one method, `Replace(ctx, store.Rows)`, and
`duckstore` implements it; `snapshot` declares a `StoreProbe` port (`Path`, `Exists`) that the
same `duckstore.Store` also implements. `cmd/quarry` builds one `duckstore.Store` and wires it
into both, with a compile-time guard for each port.

## Consequences

- **Positive:** Later phases read the store through `internal/store` and `duckstore` without importing a feature package.
- **Positive:** `snapshot`, `cli` and the importer's tests use the row types without linking DuckDB (checked with `go list -deps -test`).
- **Positive:** The single-method port means the importer never holds a half-built store; validation runs on the mapped `store.Rows` before `Replace` is called, so a failed check never creates a partial file.
- **Negative:** Two packages where the original ruling named one; a reader must know the types and the adapter live apart.
- **Trade-off:** Checks run on the in-memory rows quarry is about to write, not on the DuckDB file after writing. `duckstore`'s own round-trip tests carry the proof that what is written equals what was mapped.
- **Follow-up:** Reads of the store, and the `report` package that consumes them, are decided in [ADR-003](003-duckstore-owns-the-read-side.md).
