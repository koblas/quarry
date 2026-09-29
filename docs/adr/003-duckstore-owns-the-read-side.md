# ADR-003: duckstore owns the read side of the store; report declares the port

## Status

Accepted

## Context

Phase 2 adds commands that only read the store (`status`, `accounts`, `sql`). They need one
place that opens the store read-only, runs the queries, and later checks the store's format
and locks the connection down, so those rules are written once and not per command. The
read commands must not link the DuckDB driver into `cli` or `report` (the driver's link step
has run out of memory on small CI runners), and a feature package may not import another
feature package, so they cannot borrow the importer's or `snapshot`'s store access.

## Decision

`internal/store/duckstore` owns every read: it opens the store file read-only through one
unexported `(*Store).openRead`, runs the queries, and closes the connection before returning.
The values a read returns are driver-free types in `internal/store` (`store.Status` first).
`internal/report` is the feature package for the read commands; it declares the `Store` port
it consumes (one method per read) and `*duckstore.Store` implements it, guarded at compile
time in `cmd/quarry`. `cmd/quarry` wires the adapter into `report.Server`; `internal/cli` and
`internal/report` link no DuckDB code. A read never creates the store or its directory.

## Consequences

- **Positive:** The format check, the read-only lockdown and the missing-store refusals each land in `openRead` once and cover every read command.
- **Positive:** `cli` and `report` stay driver-free, checked with `go list -deps -test ./internal/cli ./internal/report`.
- **Negative:** `*duckstore.Store` now has two jobs, building the store and answering reads; `report.Store` grows one method per read and `duckstore` one implementation each.
- **Trade-off:** Each read opens and closes its own connection, which costs a little per command but means no read can hold the store open while a later `sync` swaps it.
