---
id: SCENARIO-11
status: open
---

# SCENARIO-11: Same-second runs do not collide (+ SCENARIO-12 folded)

Cadence: test-first — the collision reservation is an exclusive-create adapter claim
(`build.md` → *Build cadence*, "Adapters with atomicity or exclusive-create claim"). The
sweep and the `cmd/quarry` wiring stay code-first within this scenario.
Acceptance test: `internal/snapshot/sync_test.go` `Test_sync_appends_a_suffix_when_the_current_second_already_has_a_snapshot`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_test.go` `Test_run_removes_leftover_partials_silently_before_syncing`
Narrow loop: `go test ./internal/snapshot/... ./cmd/quarry/... -run 'Suffix|Backup_reserves|Backup_returns|Sweep|Leftover'`
Mutation checks: final-existence check in `dirDestination.Backup`'s retry loop → acceptance test goes red; `FinalPaths` call moved back before `Backup` in `Sync` → the on-disk `_2.json` path-fields assertion goes red; leftover-sweep regex loosened to match anything → the control-file test goes red; `-wal`/`-shm`/`-journal` companion match dropped → the companion test goes red.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/snapshot/ports.go:25-43` `Destination.Backup`, `Destination.FinalPaths` — change `Backup`'s signature to `(ctx, src, name) (partial, resolvedName string, err error)`; drop `FinalPaths`'s "assuming no collision suffix" doc (it now receives the already-resolved name)
- [ ] Step 2: `internal/snapshot/destination.go:36-50`, `sync_faults_test.go:142-160,283-326` — update `dirDestination`, `fixedPathDestination`, `partialFaultDestination` to the new signature (pass `name`/`f.snapshotPath` through unchanged as `resolvedName` for now — no suffix logic yet, just enough to compile)
- [ ] Step 3: `internal/snapshot/sync_test.go` (new test) `Test_sync_appends_a_suffix_when_the_current_second_already_has_a_snapshot` — inside `testing/synctest.Test`, build `v9fixture.OpenBundle`/`v9.Reference` *before* entering the bubble, call `srv.Sync` twice inside it (frozen fake clock gives both calls the same `time.Now()`, no manual timestamp needed); assert first pair's bytes and mtime unchanged, second pair named `<name>_2`, and the committed `_2.json`'s `snapshot.path`/`snapshot.manifest` fields carry `_2`. Must fail at the suffix/mtime assertions, not at compile.

### Build
- [ ] Step 4: `internal/snapshot/destination.go:36-50` `(*dirDestination).Backup` — reservation loop: for candidate `name`, `name_2`, `name_3`, ...: `atomicfile.Create` the candidate's sqlite partial (the reservation token); `errors.Is(err, fs.ErrExist)` → next candidate, any other create error → return immediately; on success, `os.Stat` *both* candidate finals (sqlite and json) — either exists → remove own partial, next candidate; else back up into it and return `(partial, candidate, nil)`. No iteration cap. Test-first: `Test_dirDestination_backup_reserves_the_next_suffix_when_the_current_second_is_taken` (call `Backup` twice with the *same literal name string*, no clock — asserts `_2`) and a `_3` case; fault test `Test_dirDestination_backup_returns_immediately_when_the_partial_cannot_be_created_for_a_reason_other_than_a_collision` (chmod dir `0500` before a repeat call)
- [ ] Step 5: `internal/snapshot/snapshot.go:125-131,153` — move `destination.FinalPaths(name)` to *after* `destination.Backup` returns, calling it with the returned resolved name; pass that same resolved name (not the original) to `destination.WriteManifest`
- [ ] Step 6: `internal/snapshot/destination.go:29-34` `(*dirDestination).Prepare` — after `MkdirAll`, sweep `d.dir` for regular files matching `^\.\d{8}T\d{6}Z(_\d+)?\.(sqlite|json)\.partial(-journal|-wal|-shm)?$` and remove them; `os.ReadDir`/`os.Remove` failures are swallowed (best-effort, no return error) — grep `internal/platform/sqlite` `journal_mode` handling confirms which companion suffixes a crashed backup can leave. Test: seed a matching partial + each companion suffix, `.DS_Store`, `.notes.partial`, `x.sqlite.partial` (no leading dot), and a real committed pair; assert only the matching ones are gone. Fault test: `chmod` the dir unreadable before `Prepare` — sweep no-ops, `Prepare` still succeeds
- [ ] Step 7: `cmd/quarry/run_test.go` (new test) `Test_run_removes_leftover_partials_silently_before_syncing` — pre-create a leftover `.<name>.sqlite.partial` (+ a `-wal` companion) under the snapshots dir before calling `run`; assert exit 0, stdout is the normal success block, stderr empty, and the leftovers are gone

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `Backup`, `FinalPaths`, `Prepare`, and the new sweep helper (1-2 lines each, no history)

### Verify
- [ ] Step 9: full verification + `spec-check.py phase0-snapshot` → tick SCENARIO-11 and SCENARIO-12 with their acceptance tests

## Handoff

**Binding decisions:**
- `Destination.Backup` now returns the resolved (possibly suffixed) name; `Sync` derives `FinalPaths`/`WriteManifest`'s name from that return, never from the pre-collision `name` it computed from `time.Now()`.
- Collision safety depends on `Sync`'s existing commit order staying `CommitManifest` before `CommitSnapshot`, and `atomicfile.Commit`'s link-then-remove order: a rival's partial is only removed *after* both its finals exist. Reordering either breaks the reservation loop's race-safety with no test going red — the check-both-finals-after-token-create step in `dirDestination.Backup` is what relies on it.
- No cap on the suffix retry loop (a cap is a numeric bound needing bound tests and unruled refusal copy).
- Leftover sweep is best-effort and silent: a `ReadDir`/`Remove` failure never fails `Prepare` or `Sync`.

**Left unbuilt — flagged for orchestrator ruling before dispatch:**
- **Sweep vs. concurrent syncs.** A second `Sync`'s startup sweep can delete a *first*, still-in-flight `Sync`'s partial (the sweep pattern matches any quarry partial, not just stale ones), letting the second reuse the first's name and producing a constructible failure in the first run (generic error, or a misleading content refusal) plus a `Discard` that removes the second run's file. This plan implements the plain reading of "leftovers … removed silently at start" as written in `specification.md`'s Files-on-disk table; it does not resolve the conflict with the "concurrent syncs: exclusive create; no lock file" row. Candidate mitigations (mtime-age threshold on the sweep, or an flock — untested against SQLite's own fcntl locks on the same file) are not implemented here. `product-vision`/orchestrator must rule before a developer runs this plan.

**Traps:**
- The reservation token is the *sqlite* partial's `O_EXCL` create, not a separate lock file — `WriteManifest` never needs its own retry, because it only ever runs with a name `Backup` already proved free of both finals.
- `synctest`'s fake clock only helps because `Sync`'s two calls happen back-to-back with nothing between them that blocks — if a future change adds a blocking call before `time.Now()`, the two `Sync` calls could see different frozen instants; the adapter-level `Backup` tests (same literal name string, no clock) exist as a synctest-independent fallback and should keep passing regardless.
