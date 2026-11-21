# Review Report — round 1

### Target
Changed files, e3367533fc2ed6a1a581a1a8d494c731e93f9145..HEAD (21 Go files + spec docs).

### Triggered reviewers
- arch-reviewer: cmd/**/*.go, internal/**/*.go
- correctness-reviewer: cmd/**/*.go, internal/**/*.go
- refactor-advisor: cmd/**/*.go, internal/**/*.go
- test-reviewer: **/*_test.go

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no .claude/** files

### Gate inputs
- uncovered-diff: 0 uncovered added lines; 2 declared unreachable (pre-existing, snapshot.go:325,334)
- test-stats: TOTAL 1580 (+33)
- mutation-sample: 20 sampled of 21 — 18 killed, 2 survived, 0 non-viable, 0 timed out; 851s
- spec-check --run: OK

### BLOCKER
- correctness-reviewer: `internal/importer/entities.go:50` — `resolveEntities` keeps the last Z_ENT for a duplicated Z_NAME (`found[name] = ent`); import filters on it and imports an empty result, exit 0. Reproduced via `sync --quicken` with a duplicated CategoryTag row (category lost silently); same for every looked-up entity. Pre-existing (line 44 at base) but in a rewritten function whose new doc claims duplicates stay visible. Fix: refuse via `*UnmappableError` per spec `## Surface & Copy` → *Duplicated looked-up entity* (mid-feature copy ruling).

### MAJOR
none

### MINOR
- correctness-reviewer: `internal/snapshot/resolve.go:54` — two reference columns resolving onto one (table, column) key overwrite `written[key]` and append the column twice. No constructible failure on the current reference. Fix: skip an already-present column; keep first `written` entry.
- correctness-reviewer + test-reviewer: `internal/importer/splits.go:116-118` — three `ok` results discarded; partly resolved query possible. Source of the `keepName` mutation survivor. Fix: when any `ok` is false use all three literal names.
- refactor-advisor + arch-reviewer (NIT) + test-reviewer (survivor): `internal/snapshot/resolve.go:12-15` vs `internal/importer/importer.go:61` — unset resolver handled two ways; snapshot's nil branch is an equivalent mutant (`resolve.go:15`). Fix: default `resolver` to identity in `snapshot.NewServer`, drop nil branch.
- refactor-advisor: `internal/quicken/v9/v9fixture/builder.go:290-319` + `internal/quicken/v9/entities.go:175-184` — schema-numbered entity list written out several times (`referenceEntities`, `Ent*`, `ddl` literal, `entities` literal); a miss silently stops renaming. Fix: build `ddl` from `entities` over one shared list.
- refactor-advisor: `internal/importer/entities.go:37-70` — `resolveEntities` returns three unnamed values; `found` redundant with `numbers`. Fix: return a struct or `numbers` only.
- refactor-advisor: `internal/quicken/v9/v9fixture/renumber.go:469-501` `renumberSchema` — Compose method; placeholder rename pair repeated. Fix: `rename` struct + helper.
- test-reviewer: `internal/importer/splits_test.go:~268` — no-resolver control asserts only `ErrorContains "read split tags"`. Fix: also assert `no such table: Z_15USERTAGS`.

### NIT
- correctness-reviewer + test-reviewer: `internal/quicken/v9/v9fixture/renumber.go:~40` — `context.Background()` → `tb.Context()`.
- arch-reviewer + refactor-advisor: `EntityResolver` declared in both `importer` and `snapshot` — compliant consumer-side type; optionally shorten one doc to point at `v9.ResolveName`.
- arch-reviewer: `internal/importer/splits.go:114` — Quicken literal names in importer mirror `v9` map. Pre-existing; optional.
- refactor-advisor: `builder.go:354` `shiftFrom` lacks doc; `resolve.go:574-578` hand-sorted keys → `slices.Sorted(maps.Keys(...))`; `splits.go` doc could say why dropping `ok` is safe.
- test-reviewer: `cmd/quarry/run_renumbering_test.go:17` `execOn` duplicates importer helper; `referenceLabel` lacks doc.

### Strengths
- Fixture renames through production `ResolveName`, with literal expectations in `builder_test.go`/`entities_test.go` closing the symmetric-bug gap.
- `importResolvingEntities` leaves shared `importInto` untouched (avoids UserTag-override trap).
- Real-file check (correctness): resolved reference vs live snapshot `20261121T111911Z` → verified, 0 missing; importer query `SELECT Z_16CASHFLOWTRANSACTIONENTRIES, Z_77USERTAGS FROM Z_16USERTAGS` runs (0 rows in user's file).

### Verdict: BLOCKED
1 BLOCKER (duplicated looked-up entity, importer).
