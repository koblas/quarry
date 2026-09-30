---
id: SCENARIO-16
status: open
---

# SCENARIO-16: snapshots lists newest first, marks the store's snapshot, and totals the size (absorbs SCENARIO-03, 17, 19, 20, 34)

Cadence: code-first — a read-only listing and one copy change; no write-safety guard, no atomic adapter, no bug fix
Acceptance test: `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_lists_newest_first_marks_the_stores_snapshot_and_totals_the_size`
Acceptance test (SCENARIO-03, folded): `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_warns_about_an_unknown_config_key_and_still_lists`
Acceptance test (SCENARIO-17, folded): `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_marks_a_schema_mismatch_and_a_missing_manifest`
Acceptance test (SCENARIO-19, folded): `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_with_none_taken_yet_says_how_to_take_one`
Acceptance test (SCENARIO-20, folded): `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_warns_when_the_store_cannot_be_read`
Acceptance test (SCENARIO-34, folded): `cmd/quarry/run_from_refusals_test.go` `Test_run_sync_from_an_unknown_id_points_at_quarry_snapshots`
Narrow loop: `go test ./internal/snapshot/ ./internal/store/duckstore/ ./internal/cli/ ./cmd/quarry/ -run '(?i)snapshot|_list_|built_from|unknown_id|id_form|help|usage'`
Mutation checks: order taken from manifest `taken_at` instead of the ID → `Test_list_orders_by_id_even_when_taken_at_disagrees` | `_N` compared as text → `Test_list_orders_a_numeric_suffix_by_value` | store mark compared without `filepath.EvalSymlinks` → `Test_list_marks_the_stores_snapshot_recorded_through_a_symlink` | `store` arm moved below `no manifest` / `schema differs` → `Test_snapshotStatus_puts_store_first` | total skipping a row that has no manifest → `Test_list_totals_every_snapshot_including_one_with_no_manifest` | identity keyed on the ID stem, so an orphan `.json` lists → `Test_list_ignores_everything_that_is_not_a_snapshot_file`
Runs: A (1-3) | B1 (4-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 4 batches, 1 feature package (snapshot) + store/duckstore + cli + cmd; absorbs 03, 17, 19, 20, 34. If it outgrows 4 batches, SCENARIO-34 (step 7's copy half) moves to SCENARIO-21; 17, 19, 20 are branches of code 16 writes and stay

Contract (specification.md `### quarry snapshots`, P2c-5, P2c-6, surface #3): `quarry snapshots` prints the ruled table on stdout, exit 0, also when the folder is missing or empty (header only, plus `quarry: no snapshots in <snapshots> yet; run quarry sync to take one`) and when the store is unreadable (`quarry: warning: cannot tell which snapshot the store was built from: <R3 reason>`, nothing marked). Exit 1, stdout empty: folder unreadable (`quarry: cannot read <snapshots>: <G1 reason>`), C1/C2/C2q/C2r/C4, HOME unset, stdout write failure. Exit 2: `quarry snapshots list` with the ruled usage line. `quarry sync --from <unknown id>` exits 1 with surface #3's line.

Port survey: `snapshot.StoreProbe` (`ports.go:63-70`) — production calls are `Path` (`import.go:116,125,145,180`) and `Exists` (`:117`); it gains `BuiltFrom`. Implementers (grep; LSP resolved into a sibling worktree, see Traps): `duckstore.Store` (`duckstore.go:76`, guard `cmd/quarry/run.go:28`) and `fakeStoreProbe` (`sync_and_import_test.go:59-66`). The folder read gets no port: `os.ReadDir`, `DirEntry.Info`, `os.ReadFile` (through `readManifest`) and `filepath.EvalSymlinks` stay on `os`; their faults are real file modes.

Unruled outcomes — each default below reuses ruled copy only; the orchestrator confirms them (scoped `product-vision`) before run B1:
- a listed `.sqlite` whose stat fails (folder readable, not searchable): default the folder-unreadable line, exit 1
- no snapshots AND store unreadable: default the no-snapshots line, then the warning
- manifest decodes but `taken_at` does not parse or `source` is empty: default that cell `unknown`, Status from `schema.verified` as usual
- store of this format with no import run: default the warning, reason `expected exactly one import run, found 0` (the R3 zero-run phrase)
- store of another format (R2) naming no snapshot: default silent, nothing marked
- `snapshots` interrupted: no ruled copy; default no special case (a cancelled store read ends as the warning, exit 0)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_lists_newest_first_marks_the_stores_snapshot_and_totals_the_size` — through `run`; a fixture helper writes `<id>.sqlite` as sparse files of distinct sizes, each at least 0.1 MB, plus manifests through `snapshot.Manifest.Encode`; the store names the middle snapshot through `replaceStore` (`run_helpers_test.go:76-80`) or `editStore` (`run_read_refusals_test.go:190-197`); whole stdout asserted, Taken through `time.Local` as `run_status_test.go:48`
- [x] Step 2: same file, the four folded tests named above (03: `writeConfig` `run_config_test.go:41-46`; 20: a text file at `storePathUnder(home)`); `run_from_refusals_test.go:44-58` — the `an unknown ID` row leaves the table for `Test_run_sync_from_an_unknown_id_points_at_quarry_snapshots`
- [x] Step 3: stubs so every test above fails at its own assertion — `snapshot/ports.go:63-70` `StoreProbe.BuiltFrom` + `duckstore` `(*Store).BuiltFrom` + the fake `sync_and_import_test.go:59-66`; new `snapshot/list.go` `Listing`, `(*Server).List`; `cli/run.go:17-39` `SnapshotsFactory`, `Env.NewSnapshots`; new `cli/snapshots.go` `newSnapshotsCommand` (ruled Short) registered at `cli/root.go:27-32`; `cmd/quarry/run.go:44-71,131-141` `newSnapshotsFactory` (home, snapshots folder, store probe; no reference, no importer) in `defaultEnv`; root-help pin `run_status_test.go:119-126` gains the `snapshots` row

### Build
- [ ] Step 4: `snapshot/list.go` — identity (name pattern beside `destination.go:21`, regular files only), the one ordering func, size from stat, manifest through `readManifest` (`from.go:135-146`), total, missing folder vs unreadable folder (`osreason.Reason`). New `snapshot/list_test.go`: the three order and total tests on the `Mutation checks:` line, plus bare-before-`_2` and a suffix too long for an integer; `Test_list_ignores_everything_that_is_not_a_snapshot_file` (orphan `.json`, partial, stray, subdirectory and symlink named like a snapshot; one real pair as control); size on disk wins over the manifest's `bytes`; manifest missing / unreadable / not JSON / a directory → a row with no manifest, never a failed listing; folder missing, folder empty, and folder holding only non-snapshots (a partial, an orphan `.json`) → the same empty listing and no-snapshots line, folder not created; folder unreadable, a file at the folder's path, and the stat fault → the refusal
- [ ] Step 5: new `duckstore/built_from.go` `(*Store).BuiltFrom` — `openRead` (`duckstore.go:151-165`), `snapshotPathQuery` (`:175`), faults through `openFault` (`:179-198`), no row as `status.go:61-63`; new `built_from_test.go` (tests named `Test_built_from_…`): latest of two runs, Missing, OtherFormat carrying its path, NotDuckDB, Permission, Locked, no run, query fault through `spyReadDB` (`fakes_test.go:43`). `snapshot/list.go` — classify the fault, mark after `EvalSymlinks` on both sides, the warning through `UnreadableReason`; `list_test.go`: no probe and no store mark nothing silently; the symlink test, with `--from <path>` outside the folder as its control; snapshot deleted by hand keeps the recorded path, marks nothing, no warning; a store of another format still marks; one row per R3 fault → the ruled warning, nothing marked
- [ ] Step 6: `cli/snapshots.go` — `Args` returning the whole ruled usage line as a `UsageError`, hint included (`Execute` appends the hint only to other errors, `cli/run.go:59-65`; not `noArgs`, `errors.go:15-20`), config loaded first as `sync.go:80-86`, Long, Example; new `cli/render_snapshots.go` — table, Total, status cell. New `cli/render_snapshots_internal_test.go` under `useZone` (`render_status_internal_test.go:13-20`): the spec's table verbatim, header only, no trailing space on a blank-Status row or the Total row, Source as base name, `unknown` cells, `Test_snapshotStatus_puts_store_first`. New `cli/snapshots_test.go`: Short, Long, Example verbatim (as `report_help_test.go:11-63`). `run_snapshots_test.go`: `snapshots list` exit 2; one row each C1/C2/C2q/C2r/C4; a `quicken.path` missing on disk changes nothing (the C2r row is its control); HOME unset names `snapshots`; stdout write failure; folder unreadable (`skipAsRoot`); no store → nothing marked, stderr empty; both stderr prefixes. `run_usage_test.go:217-274` gains `snapshots --help` and `snapshots list`
- [ ] Step 7: `snapshot/from.go:116-120` `idNotFoundRefusal` — surface #3's copy; pin `import_from_test.go:371-380`. `cmd/quarry/run.go:63,110-113` — one `snapshotsDirUnder` for both factories; the factory's own home refusal tested with a stubbed `Env.LoadConfig`

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments within budget on `List`, `Listing`, `StoreProbe` (`ports.go:63-64`), `BuiltFrom`, `newRootCommand` (`root.go:7-9`), `snapshot/doc.go`, `duckstore/doc.go`

### Verify
- [ ] Step 9: full verification + `.claude/scripts/spec-check.py phase2c-snapshots-config` → tick SCENARIO-16, and 03, 17, 19, 20, 34 as delivered by SCENARIO-16, each with its acceptance test; rewrite `STATE.md` (drop the three Left-unbuilt rows this closes, keep the `prune` half of the `quicken.path` row)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Listing is `(*snapshot.Server).List` in `internal/snapshot/list.go`, with one name pattern and one ordering func — P2c-5: SCENARIO-21 and 29 select what to delete from this order, never from a second sort
- `_N` is compared as digits (length, then text), never parsed — a suffix of any length matches the pattern, and a parse would add a fault branch
- `StoreProbe.BuiltFrom(ctx)` returns the latest run's snapshot path or a `*store.OpenError`, Missing and OtherFormat included. `snapshot` classifies once, with `if`s: Missing → no store; OtherFormat → its `SnapshotPath`; anything else → the fault. `List` turns that fault into the warning; SCENARIO-21 turns the same fault into its refusal (R3 → nothing deleted)
- The mark is `EvalSymlinks` on both sides; a store side that does not resolve marks nothing and keeps the recorded path — the edge rows "deleted by hand" and "`--from <path>` outside the folder" both need `store_snapshot` to survive
- `Listing` carries the folder (absolute), rows newest first (ID, path, bytes, `*Manifest` with nil = no manifest, store flag), the recorded store path unresolved, and the total — SCENARIO-18 adds only `renderSnapshotsJSON` and the `--json` branch; `keep` is `cfg.Keep`, `store_snapshot.id` is `snapshotID`
- The no-snapshots line and the store warning stay separately addressable on `Listing`: stderr prefixes differ (`quarry: ` vs `quarry: warning: `) and `emit` (`output.go:45-53`) takes one prefix
- Status words and their precedence live in `internal/cli` (`snapshotStatus`); `snapshot` hands over facts, as `--json` needs them
- `Env.NewSnapshots` builds a Server with no reference and no importer; `snapshots prune` (21, 24) reuses it, sync's auto-prune (29) stays on `NewServer`
- `snapshots` keeps an explicit `Args` — once 21 adds the `prune` child, cobra accepts any positional on a parent whose `Args` is nil

**Left unbuilt** — named so nobody assumes it exists:
- `renderSnapshotsJSON` and the `--json` branch — SCENARIO-18; until then `quarry snapshots --json` prints the table
- `snapshots prune` — SCENARIO-21; the Example and the usage line already name it
- interrupt handling on `snapshots`, and the prune meaning of "another-format store naming no snapshot" — unruled (see above); 21 must rule the second before it deletes

**Traps** — things that look right and are not:
- LSP `goToImplementation` answered from a sibling worktree (`../agent-…/`); take implementers and line numbers from grep
- `snapshotPath()` (`duckstore.go:244-259`) swallows every error for R2's hint; `BuiltFrom` must not call it. It shares `snapshotPathQuery`'s text, and `spyReadDB` routes faults by query text
- `UnreadableReason` is empty for Missing and OtherFormat: classify those first or the warning ends in a colon
- `renderTable` (`render_table.go:20-51`) always writes a caption and right-aligns every column after the second; Source is left-aligned — build on `padRight` / `padLeft` / `accountsColumnGap` (`render_accounts.go:14,75-82`)
- Tiny fixtures all print `0.0 MB`, so a Total assertion cannot fail; with HOME a raw `t.TempDir()` both paths match unresolved, so the symlink test needs an explicit alias
- `newServerFactory` hardcodes `resolveHome("sync")` and loads the Quicken reference; routing `snapshots` through it leaks `sync interrupted; ...` copy

## Phase report

Run A (steps 1-3) done. Every test below fails at its own assertion (stdout/stderr empty vs ruled text; the `--from` test on the old copy `check the ID passed to --from`).

Files:
- `cmd/quarry/run_snapshots_test.go` (new): the acceptance test and four folded tests; helpers `pinLocalZone` (sets `time.Local` to UTC-4 "EDT", so tables are literals), `writeSnapshots(t, home, ...snapshotFixture)` (sparse `.sqlite`, manifest via `Encode`; zero `taken` = no manifest), `buildStoreFrom(t, home, snapshotPath)` (via `replaceStore` + `spendRows(nil)`), `runSnapshots(t)`, `olderPair()`. Sizes `oldestBytes/middleBytes/newestBytes` sum to 6.72 MB whose rounded parts sum to 6.6 (Total is discriminating).
- `cmd/quarry/run_from_refusals_test.go`: the `an unknown ID` row is out of the table; new `Test_run_sync_from_an_unknown_id_points_at_quarry_snapshots` (red until step 7 changes `idNotFoundRefusal`; `internal/snapshot/import_from_test.go:371-380` still pins the old copy).
- `cmd/quarry/run_status_test.go`: root-help pin gains the `snapshots` row (green with the stub).
- `internal/snapshot/ports.go` `StoreProbe.BuiltFrom(ctx) (string, error)` (stub implementers: `duckstore/built_from.go`, `fakeStoreProbe` in `sync_and_import_test.go`).
- `internal/snapshot/list.go` (new stub): `Entry{ID,Path,Bytes,Manifest,Store}`, `Listing{Dir,Entries,StorePath,TotalBytes,NoSnapshots,StoreWarning}`, `(*Server).List` returns `Listing{}`. `NoSnapshots` / `StoreWarning` are the two separately addressable strings (whole text after the stderr prefix); B1 may reshape freely.
- `internal/cli/run.go` `SnapshotsFactory func(ctx, command string)` (takes `command` like `ReportFactory`, for the home refusal), `Env.NewSnapshots`; `internal/cli/root.go` registers `newSnapshotsCommand(env.NewSnapshots, env.LoadConfig)`; `internal/cli/snapshots.go` stub: Use + ruled Short, `RunE` returns nil (no `Args`, `Long`, `Example` yet: step 6).
- `cmd/quarry/run.go` `newSnapshotsFactory(storeOpts...)`: home, `WithSnapshotDir`, `WithHome`, `WithStoreProbe(duckstore.New(...))`; in `defaultEnv`. `snapshotsDirUnder` is still to be extracted (step 7).

Next (B1, steps 4-5): everything is stubs; `go vet ./...` and `golangci-lint run ./...` are clean (0 issues). Do not re-add the `time.Local` pin per test; reuse `pinLocalZone`. Tables in the tests are literals built by hand: if one mismatches after B1, check the test's spacing before the production alignment. Empty-listing table is header only, `ID  Taken  Size  Source  Status` (Size right-aligned to the widest of header, rows and Total).
