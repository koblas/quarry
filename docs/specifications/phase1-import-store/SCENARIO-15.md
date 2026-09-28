---
id: SCENARIO-15
status: open
---

# SCENARIO-15: --from refuses input that is not a usable snapshot (absorbs 17)

Size verdict: OWNS A RUN — one package (`internal/snapshot`), one `When` (Outline), command tests only.

**Blocked on a copy ruling before dispatch** (orchestrator → scoped `product-vision`), does not block planning:
1. F4 naming when the *manifest* (not the snapshot) fails an OS read — spec's F4 example is generic `~/x.sqlite`. Proposed: name whichever path actually failed, via `errors.As(err, &pathErr); pathErr.Path`.
2. `fromRefusal`'s residual arm (SQLite content faults after a successful open that are neither `IsNotADB` nor no-accounts — e.g. `snap.Schema` failure): no ruled text, unreachable by any fixture today. Proposed: fold into the F4 shape rather than invent a sixth reason.
The check-order line already rules integrity-check failures into F3 `not a SQLite database` — no separate ruling needed there.

Cadence: code-first — no bug reproducing a `Failure:`, no write-safety guard, no atomic adapter.
Acceptance test: `cmd/quarry/run_from_refusals_test.go` `Test_run_refuses_from_input_that_is_not_a_usable_snapshot`
Acceptance test (SCENARIO-17, folded): `cmd/quarry/run_schema_test.go` `Test_run_reports_a_schema_mismatch_with_from`
Narrow loop: `go test ./internal/snapshot/ ./cmd/quarry/ -run 'ImportFrom|import_from|resolveFrom|From|from|schema_mismatch'`
Mutation checks:
- path pre-check runs before `readManifest` → moving it after reddens the F1/F1b/F2/F2b rows (they'd show the F3 frame) in `Test_run_refuses_from_input_that_is_not_a_usable_snapshot`
- `resolveFrom`'s form flag selects F1 vs F1b → inverting it swaps those two rows in the same test
- F2b's `.quicken`+dir arm precedes generic F2 → dropping or loosening it swaps the F2/F2b rows in the same test
- F4's `*fs.PathError` arm → narrowing it to the F3 default reddens the F4 row in the same test
- `sqlite.IsNotADB` arm → dropping it reddens `Test_import_from_refuses_without_importing_when_the_snapshot_is_not_sqlite`

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_from_refusals_test.go` (new, pattern: `run_bundle_refusals_test.go:1-30`) `Test_run_refuses_from_input_that_is_not_a_usable_snapshot` — table over the Outline's 7 rows (F1 nonexistent path, F1b unknown ID, F2 plain directory, F2b `.quicken` bundle via `v9fixture.OpenBundle`, F3 missing manifest, F4 chmod-0000 snapshot, F5 already-covered hash-changed regression check); each row asserts exact stderr, empty stdout, exit 1, `quarry.duckdb` absent. F1/F1b/F2/F2b rows carry **no manifest file** at all — that absence is what proves the path pre-check runs first.
- [ ] Step 2: signature-only stub — `internal/snapshot/from.go:67` `resolveFrom` gains an `isPath bool` return; `from_internal_test.go:37` updated to compile against it (no new assertions yet)

### Build
- [ ] Step 3: `internal/snapshot/from.go:19-27` `ImportFrom` — head `os.Stat(snapshotPath)` step using `resolveFrom`'s `isPath`: `fs.ErrNotExist` → F1 (`isPath`) or F1b (`!isPath`, names `homepath.Abbreviate(home, s.snapshotDir)`); other stat error → F4; `info.IsDir() && strings.EqualFold(filepath.Ext(snapshotPath), ".quicken")` → F2b; `!info.Mode().IsRegular()` → F2 (names `homepath.Abbreviate(home, s.snapshotDir)` too). Route through `FailureOutcome`. New refusal builders beside `fromRefusal`. Unit tests: F1/F1b/F2/F2b each via `srv.ImportFrom`, plus a regular-file control (proves F2 doesn't fire on a real snapshot).
- [ ] Step 4: `internal/snapshot/from.go:80-90` `readManifest` — tag its own two failures at the source (missing vs. any other OS read fault) instead of classifying `fs.ErrNotExist` generically downstream; decode failure stays distinguishable via `*json.SyntaxError`/`*json.UnmarshalTypeError`. Unit test: manifest present but unreadable (chmod) → F4, naming the manifest path.
- [ ] Step 5: `internal/snapshot/from.go:94-107` `fromRefusal` — introduce a closed `notSnapshotReason` string type with the five F3 constants (`no .json manifest next to it`, `its manifest is not readable JSON`, `not a SQLite database` via `sqlite.IsNotADB`, `it has no accounts table`, `it has no accounts`) and one builder taking only that type, so `causeText(err)` (a plain `string`) cannot compile into it — this is what retires the interim frame. Add the F4 arm (Step 4's tag, plus `hashFile`'s own OS-read faults) using one builder keyed off `errors.As(err, &pathErr)`, naming `pathErr.Path`. Residual `default:` arm per the blocked ruling (proposal: fold into the F4 shape). Update `import_from_test.go:273-306` (drop the now-impossible "snapshot is a directory" sub-case — Step 3 catches it before `readManifest`), `:308-320`, `:322-336`, `:338-362` to assert the exact final F3/F4 text instead of `ErrorIs`/`ErrorAs` typed checks.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `resolveFrom`, `fromRefusal`, new builders (≤2 lines, unexported)

### Verify
- [ ] Step 7: full verification per `agent-briefs.md`. Evidence for "no F1/F1b/F2/F2b/F4 case reaches the interim frame": `grep -n 'is not a quarry snapshot' internal/snapshot/*.go | grep -v _test` → exactly one line (the closed-type builder). Positive control, same pipeline on `'has changed since quarry took it'` (F5's own fixed string) → exactly one line, proving the grep isn't blind. Note what the grep can't see (a computed reason string) — that's what the closed type, not the grep, rules out. `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-15 (+17) with their acceptance tests

## Handoff

**Binding decisions:**
- `fromRefusal` is no longer the *only* --from refusal source — STATE's SCENARIO-03 "ONE classifier" decision is amended: a head `os.Stat` pre-check in `ImportFrom` now produces F1/F1b/F2/F2b before `readManifest` (and thus before `fromRefusal`) ever runs. `fromRefusal` still owns every post-manifest-read classification (F3, F4, M1b stays via `fromMismatchError`, untouched).
- F3's five reasons are a closed `notSnapshotReason` type — a computed `causeText(err)` cannot reach that frame; any future F3-shaped reason must be added as a sixth named constant, ruled first.

**Left unbuilt:**
- The `fromRefusal` residual `default:` arm's exact copy — blocked on the copy ruling above; do not ship it in the F3 frame.
- F4's naming when the manifest (not snapshot) fails to read — blocked on the same ruling.

**Traps:**
- `import_from_test.go:285-288`'s "snapshot is a directory" sub-case asserts `ErrorIs(syscall.EISDIR)` — that path never reaches `hashFile` anymore (Step 3's stat catches it first with no errno at all); rewrite, don't fake a cause.
- STATE's SCENARIO-03 mutation note "moving hash+F5 after `inspectContent` reddens the snapshot-missing / directory rows" is now stale — only `…_reports_a_changed_snapshot_that_no_longer_opens_as_changed` still proves that ordering; the snapshot-missing/directory rows are now caught by Step 3, earlier still.
- F2 and F2b's copy both need `homepath.Abbreviate(home, s.snapshotDir)` — not a literal path — matching F1b's same requirement.
