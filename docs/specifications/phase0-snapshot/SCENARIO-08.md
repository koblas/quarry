---
id: SCENARIO-08
status: open
---

# SCENARIO-08: Snapshot content rejected (R10 integrity, R11 no ZACCOUNT incl. 0-byte data, R12 empty ZACCOUNT)

Size verdict: OWNS A RUN.

Cadence: test-first — write-safety guard: BR-9 ("rejected snapshot leaves nothing on disk") adds
cleanup of an already-written snapshot partial, the same exclusive-create partial lifecycle
`Destination.Backup`/`WriteManifest` already carry (`build.md` mandatory set, "write-safety
guards" + "adapters with atomicity or exclusive-create claim").
Acceptance test: `cmd/quarry/run_test.go` `Test_run_refuses_a_bundle_whose_snapshot_content_is_rejected`
Narrow loop: `go test ./cmd/quarry/... ./internal/snapshot/... ./internal/platform/sqlite/... ./internal/quicken/v9/v9fixture/... -run '(?i)sync|refuses|integrity|discard'`
Mutation checks:
- Remove the `destination.Discard(...)` call added to `Sync`'s buildManifest-failure branch → must redden the acceptance test's "snapshots dir has zero entries" assertion (any row).
- Remove/no-op the ZACCOUNT-existence check (`exists == 0`) → must redden the R11 (0-byte data) row: without it, `QueryInt("SELECT count(*) FROM ZACCOUNT")` hits the generic default-arm wrap, not R11's exact text.
- Disable the header-stripping in `IntegrityCheck` (use the raw first row unstripped) → must redden the R10 acceptance row's "stderr is exactly one line" assertion.

## Port survey — `Destination` (adding `Discard`)

Every current implementer, from `grep -rn "func.*FinalPaths" internal/`: `dirDestination`
(`internal/snapshot/destination.go`, production), `fixedPathDestination` and
`partialFaultDestination` (`internal/snapshot/sync_faults_test.go`, fakes). All three need
`Discard`: production removes the file and returns any `os.Remove` error unchanged (no
tolerance for "already gone" — nothing else ever removes a partial `Discard` is about to
touch, so that branch would be untestable dead code); `fixedPathDestination.Discard` no-ops;
`partialFaultDestination.Discard` calls through to `f.real.Discard` (so real cleanup still
happens), records the partial it was called with, then returns a configurable fault instead
of the real result when one is set.

## Empirically confirmed before this plan (do not re-derive)
- 0-byte `data`, through the *real* `Backup`+`atomicfile` chain (not just `Open`/`Probe`):
  opens, backs up to a 4096-byte valid empty partial, `IntegrityCheck` passes, and the
  `ZACCOUNT`-existence query returns 0 with no error.
- The index-corruption fixture (`newIndexCorruptedSnapshot`'s recipe) survives the real chain
  unchanged: `Backup` succeeds, the partial's `IntegrityCheck` still fails, `ZACCOUNT`'s count
  is still 1.
- `PRAGMA integrity_check`'s first ROW is **not** a single line on either corruption recipe in
  this codebase — `newIndexCorruptedSnapshot`'s and `sqlite_test.go`'s own
  `newMultiPageTestDatabase`/`corruptLastPage` both scan as `"*** in database main ***\n<one
  detail line>"` — a boilerplate header plus the real diagnostic, plus a second row
  (`"database disk image is malformed"`) that is never read.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1 (test-only, real — no stub): `internal/quicken/v9/v9fixture/fixture.go` — move `newIndexCorruptedSnapshot`/`requireOnlyIntegrityCheckFails` out of `internal/snapshot/sync_faults_test.go:186-244` into an exported `CorruptDataFile(tb, path)` (builds its own fresh, closed, non-WAL database at path, then corrupts it), whose verify helper pins (via a plain, independent `database/sql` query) that PRAGMA's raw first row starts with `"*** in database"` — confirmed true for this recipe above. Add a second exported helper building a closed (non-WAL) bundle whose `data` has the reference schema (`v9.ReferenceDDL`) and zero `ZACCOUNT` rows. Update `internal/snapshot/sync_faults_test.go` to call the moved `CorruptDataFile` (no behaviour change there yet). Update the package doc comment — no longer WAL-fixtures-only.
- [ ] Step 2: `cmd/quarry/run_test.go:332-349` (sibling: `Test_run_refuses_an_encrypted_bundle`) — new table-driven `Test_run_refuses_a_bundle_whose_snapshot_content_is_rejected`, 3 cases (R10/R11/R12), using Step 1's real fixtures (never a stub — a stub bundle has no `data` and would redden at `ResolveBundlePath`'s R6, not at Sync's content checks). Asserts exact stderr line (exactly one `\n`), empty stdout, exit 1, and `os.ReadDir(snapshotsDir)` returning zero entries (dotfiles included — the leftover partial is named `.<name>.sqlite.partial`). R10's expected text is derived by opening the corrupted `data` file with a plain, independent `database/sql` connection and running `PRAGMA integrity_check` directly — never by reading `IntegrityError.Result` back from the code under test. Must fail at its assertion (generic `sync …:` message, `.partial` left behind) before Build.
- [ ] Step 3: `internal/snapshot/ports.go:22-44` `Destination` — add `Discard(ctx, partial string) error` (signature-only); stub every implementer the port survey above lists so `go vet` is clean.

### Build
- [ ] Step 4 (adapter + its tests): `internal/snapshot/destination.go` `dirDestination` — implement `Discard`: `os.Remove(partial)`, wrapping and returning any error unchanged. `internal/snapshot/destination_test.go` (sibling: `Test_dirDestination_backup_fails_when_the_directory_is_not_writable`, line 36) — `Test_dirDestination_discard_removes_the_partial` (happy path via a real `Backup`-created partial) and `Test_dirDestination_discard_fails_when_the_partial_cannot_be_removed` (chmod the containing dir `0o500`, same technique as the sibling fault tests).
- [ ] Step 5 (platform + its test): `internal/platform/sqlite/sqlite.go:113-124` `IntegrityCheck` — return a new typed `IntegrityError{Result string}`, where `Result` is the **last physical line** of the first PRAGMA row (`strings.Split(row, "\n")`, take the final element) — a rule with no fallback branch. `internal/platform/sqlite/sqlite_test.go:84-95` `Test_integrity_check_fails_on_a_corrupted_database` — open the same corrupted file with a second, independent plain `database/sql` connection, run `PRAGMA integrity_check` there, and assert `errors.As`'s `Result` equals that row's own last line (never compare `Result` to itself); also assert the raw row does start with `"*** in database"` (confirmed true for this fixture too) and that `Result` does not contain that substring.
- [ ] Step 6 (core behaviour + its tests): `internal/snapshot/snapshot.go:168-196` `buildManifest` — before the accounts-count query, add an existence check (`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ZACCOUNT'`) returning sentinel `errNoAccountsTable` when absent; return sentinel `errNoAccounts` when the count is 0. `internal/snapshot/content_refusal.go` (new, sibling to `refusal.go`) — `contentRefusal(home, bundlePath string, err error) error`: `errors.As` into `sqlite.IntegrityError` → R10 (`%s` = `Result`); `errors.Is(errNoAccountsTable)` → R11; `errors.Is(errNoAccounts)` → R12; default arm keeps the existing `fmt.Errorf("sync %s: %w", bundlePath, err)`. Does not touch `sourceRefusal`. `internal/snapshot/snapshot.go:133-136` `Sync` — on `buildManifest` failure, call `destination.Discard(ctx, snapshotPartial)` (best-effort: its own error is discarded, mirroring `_ = source.Close()`, and never replaces the classified refusal), then return `contentRefusal(...)`. `internal/snapshot/sync_faults_test.go`: rename and strengthen `Test_sync_wraps_an_error_when_the_snapshot_fails_integrity_check` → `Test_sync_refuses_a_snapshot_that_fails_integrity_check` and `Test_sync_wraps_an_error_when_the_snapshot_has_no_accounts_table` → `Test_sync_refuses_a_snapshot_with_no_accounts_table` (repoint its fixture to a *non-empty* db lacking `ZACCOUNT` — the general case; 0-byte is Step 2's row) to assert the exact `RefusalError` text; add `Test_sync_refuses_a_snapshot_with_no_account_rows` (R12); add `Test_sync_still_returns_the_classified_refusal_when_discard_fails`, asserting the returned error is still the R10/R11/R12 text and `partialFaultDestination`'s recorded discard-path equals the path its own `Backup` returned; strengthen the existing `Test_sync_wraps_an_error_when_the_snapshot_cannot_be_opened` (line 156, `contentRefusal`'s default arm) to also assert the error is **not** a `RefusalError` (proving the default arm doesn't misclassify an unrelated failure as R10/R11/R12).
- [ ] Step 7: `cmd/quarry/run_test.go` — finish Step 2's test against the real implementation.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `Discard`, `contentRefusal`, `IntegrityError`, the two new `v9fixture` exports, and `IntegrityCheck`'s updated contract (state the last-line rule as the contract, not history).

### Verify
- [ ] Step 9: full verification (`go build`, full `go test` w/ coverage, `go test -race` on touched packages, `golangci-lint run ./...`, `uncovered-diff.py`, `test-stats.py`) + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-08 with its acceptance test.

## Handoff

**Binding decisions**:
- `Destination` gains a fourth write-side method, `Discard(ctx, partial string) error` — every future `Destination` implementer (fakes included) must add it; it is the only place a snapshot partial is removed after a successful `Backup`. `Sync` treats its error as best-effort and never lets it override a classified refusal — **deviation**: if `Discard` genuinely fails in production, R10/R12's "nothing was kept" text is then false and BR-9 is silently unmet; unruled by product-vision, flag it.
- `sqlite.IntegrityCheck` returns a typed `IntegrityError{Result string}`; `Result` is PRAGMA's first row's **last physical line**, not the raw row (which can contain an embedded `\n`) — callers must `errors.As`, never parse `Error()` text. **Deviation**: this extraction rule is new copy, unruled by product-vision.
- Content-check failures on the snapshot copy are classified by a new `contentRefusal`, kept separate from `sourceRefusal` (Source-facing only, per STATE.md) — a later scenario must not merge them.
- An empty, `0700` snapshots directory is left behind after every R10/R11/R12 refusal (`Prepare` already ran before the content check can fail) — BR-9 "nothing left on disk" is read as "no files", not "no directory". **Deviation**: flag this reading too.
- `v9fixture` now also builds non-WAL, non-open fixtures (`CorruptDataFile`, an empty-`ZACCOUNT` bundle); its package doc no longer says "WAL-mode … held open" exclusively.

**Left unbuilt**:
- `WriteManifest`/`CommitManifest`/`CommitSnapshot` failure branches in `Sync` still leak a partial on disk — `Discard` now exists for SCENARIO-13/R14 to reuse there; unowned until then.
- `_2`/`_3` collision suffix (SCENARIO-11), leftover `.partial` sweep (SCENARIO-12), R13 itself (SCENARIO-13) — unchanged, still owned there.

**Traps**:
- `CorruptDataFile`'s corruption trick only works on a database it built itself (`ZACCOUNT`'s pages near the front, a large filler index owning the last pages) — never run it against an existing WAL-open fixture's `data`; confirmed it survives the real `Backup` chain unchanged.
- A 0-byte `data` file opens, probes and backs up clean (a valid, empty 4096-byte DB) — verified through the real `Backup`/`atomicfile` chain; it must never reach `sourceRefusal`'s encrypted branch.
- `IntegrityCheck`'s first PRAGMA row is header-plus-detail, plus a second, unread row, on both corruption recipes in this codebase — never treat the raw row as "the first result line"; a test that reads its expected value off `IntegrityError.Result` proves nothing about the extraction rule.
- A stub (no real `data` file) for the R10/R12 acceptance rows reddens at `ResolveBundlePath`'s R6, not at Sync's content checks — the fixtures must be built for real before the acceptance test is written, even though that means doing test-only work inside the "Acceptance (red)" phase.
