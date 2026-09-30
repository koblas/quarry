---
id: SCENARIO-21
status: open
---

# SCENARIO-21: prune deletes all but the newest N snapshots (absorbs SCENARIO-23, 25, 26, 27)

Cadence: test-first — write-safety guards: prune deletes the user's snapshot files (protection, refuse-before-delete, delete order, file filter, interrupt)
Acceptance test: `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_deletes_all_but_the_newest_n`
Acceptance test (SCENARIO-23, folded): `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_keeps_the_stores_snapshot_when_it_is_older_than_the_newest_n`
Acceptance test (SCENARIO-25, folded): `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_refuses_when_the_store_cannot_be_read`
Acceptance test (SCENARIO-26, folded): `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_refuses_keep_0_as_a_usage_error`
Acceptance test (SCENARIO-27, folded): `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_reports_each_snapshot_it_could_not_delete_and_exits_1`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ -run '(?i)prune|_list_|usage|runWith'`
Mutation checks: store-snapshot skip in `selectPrune` → `Test_prune_never_deletes_the_stores_snapshot_when_it_is_older_than_the_newest_n` | newest-N cut in `selectPrune` (`keep-1`, then `keep+1`) → `Test_prune_keeps_exactly_the_newest_n` | `keep < 1` guard in `(*Server).Prune` → `Test_prune_deletes_nothing_for_a_keep_below_one` | `StoreUnreadable` refusal return in `Prune` → `Test_prune_deletes_nothing_when_the_store_cannot_say_which_snapshot_built_it` | `.sqlite`-before-`.json` order in the delete (swap to `.json` first) → `Test_prune_deletes_the_snapshot_file_then_its_manifest` | stop after a failed `.sqlite` remove (drop the early return) → `Test_prune_leaves_the_manifest_when_the_snapshot_file_cannot_be_deleted` | ctx check in the delete loop → `Test_prune_stops_deleting_once_interrupted` | regular-file filter `list.go:109` → `Test_prune_deletes_only_snapshot_files_inside_the_folder` | `.sqlite`-absent condition of the orphan sweep → `Test_prune_sweeps_only_orphan_manifests` | `--keep` bound in the command's `Args` → `Test_run_snapshots_prune_refuses_keep_0_as_a_usage_error`
Runs: A (1-2) | B1 (3-4) | B2 (5) | B3 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (snapshot) + cli + cmd; 4 folds

Contract (spec `### quarry snapshots prune`, copy verbatim from there): `quarry snapshots prune [--keep n]`. stdout: the `Deleted …` block only (empty when nothing was deleted). stderr + exit: usage → 2; refusal, any failed delete, interrupt → 1; else 0.

Unruled outcomes — this plan takes the default shown; the orchestrator gets a ruling before the run named. Steps 3-5 test names describe these defaults, not ruled copy: the tests follow the ruling.
- U6 (B1, safety) a recorded store path that no longer resolves (home renamed or migrated) marks nothing (`list.go:170-178`), so the same-ID snapshot in the folder is unprotected and prune can delete the store's own snapshot. Default: not changed here (protection = `Entry.Store`). A ruling that matches by ID moves `markStoreSnapshot`, the `snapshots` Status column and Step 3's guard together.
- U7 (B1) no snapshots (or no folder) and an unreadable store: two ruled rows collide (cannot-tell refusal, exit 1 / 24's `Nothing to delete: no snapshots in …`, exit 0). Default: refuse. Nothing is deleted either way.
- U1 (B1) `.sqlite` removed, `.json` remove fails (not ENOENT): snapshot counts as deleted, fault silent, leftover is an orphan the next prune sweeps. No copy.
- U2 (B2) orphan `.json` remove fails: silent, exit unaffected. No copy.
- U3 (B2) singular of the mid-delete interrupt line: proposed `snapshots prune interrupted; 1 snapshot was not deleted` (spec has the plural only).
- U4 (B2) failure + interrupt together: failure lines first, then the interrupt line; N counts selected snapshots never attempted; bare line iff none was attempted.
- U5 (B1) `.sqlite` already gone at delete time (ENOENT): ordinary failure row (`cannot delete snapshot <id>: no such file or directory`), exit 1.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_prune_test.go` — the five acceptance tests above plus `runPrune` helper; fixtures from `run_snapshots_test.go:33-98` (`writeSnapshots`, `buildStoreFrom`, `pinLocalZone`), store faults as `run_snapshots_refusals_test.go:168-180`; every test asserts the surviving and deleted `.sqlite`/`.json` files on disk, 26 included; 27 uses `runWith` with `env.NewSnapshots` stubbed (precedent `run_snapshots_refusals_test.go:239-252`) to a Server whose remove fails for the oldest `.sqlite` with `&fs.PathError{Op: "remove", Err: syscall.EACCES}`
- [ ] Step 2: `internal/snapshot/snapshot.go:26-100` `WithRemove` — signature-only option (remove func, default `os.Remove`) so Step 1 compiles; red = each test fails at its exit-code/stderr assertion (today the parent's `Args`, `snapshots.go:35-41`, refuses `prune` as a positional)

### Build
- [ ] Step 3: `internal/snapshot/prune.go` `selectPrune`, `Pruned`, `(*Server).Prune` keep guard + `internal/snapshot/prune_test.go` — selection over `Listing.Entries` in listing order (no sort), protection keyed on `Entry.Store` only; tests through `Prune` with helpers `list_test.go:24-79,336-351` and `fakeStoreProbe` `sync_and_import_test.go:61-73`: `Test_prune_keeps_exactly_the_newest_n` (count N → none, N+1 → oldest only, a `_10`/`_2` pair), `Test_prune_never_deletes_the_stores_snapshot_when_it_is_older_than_the_newest_n` (store at index N → N+1 kept; index N-1 as control), `Test_prune_protects_nothing_without_a_store_or_for_a_store_built_outside_the_folder`, `Test_prune_deletes_nothing_for_a_keep_below_one` (0, -1; 1 as control)
- [ ] Step 4: `prune.go` `(*Server).Prune` (List → interrupt → refusal → select → delete), delete pair through the `WithRemove` seam; `list.go:142-166` reasons reused, store path `homepath.Abbreviate(s.home, s.storeProbe.Path())` — `Test_prune_deletes_nothing_when_the_store_cannot_say_which_snapshot_built_it` (rows: not DuckDB, permission, locked, no import history, another version naming no path; whole refusal line; readable store as control), `Test_prune_refuses_a_folder_it_cannot_read` (List's line unchanged), `Test_prune_is_interrupted_when_the_context_ended_before_any_delete` (nil probe and faulting probe → `snapshots prune interrupted`, `ErrorIs context.Canceled`), `Test_prune_deletes_the_snapshot_file_then_its_manifest`, `Test_prune_leaves_the_manifest_when_the_snapshot_file_cannot_be_deleted` (fact: entry + `osreason.Reason` = `permission denied`; later snapshots still deleted), `Test_prune_deletes_a_snapshot_that_has_no_manifest` (ENOENT on `.json` is success), `Test_prune_counts_a_snapshot_deleted_when_only_its_manifest_cannot_be_removed` (U1), `Test_prune_reports_a_snapshot_file_that_is_already_gone` (U5)
- [ ] Step 5: `list.go:36-52,96-120` orphan manifests gathered in `snapshotFiles`' one `ReadDir` (unexported on `Listing`; pattern beside `destination.go:23-24`), swept last and only on an uninterrupted run; ctx checked before each snapshot, never inside a pair — `Test_prune_sweeps_only_orphan_manifests` (orphan gone and in neither list; survive: a kept snapshot's manifest, a directory `<id>.json`, a symlink `<id>.json`, `notes.json`, `<id>.json` beside `.<id>.sqlite.partial`), `Test_prune_ignores_an_orphan_manifest_it_cannot_remove` (U2), `Test_prune_deletes_only_snapshot_files_inside_the_folder` (survive: symlink `<old id>.sqlite` and its outside target, empty directory `<old id>.sqlite`, partials, strays, a subdirectory), `Test_prune_stops_deleting_once_interrupted` (remove cancels ctx: started pair finished, rest intact, `…; 2 snapshots were not deleted`, no sweep), `Test_prune_says_one_snapshot_was_not_deleted` (U3), `Test_prune_counts_only_unattempted_snapshots_after_a_failure_and_an_interrupt` (U4)
- [ ] Step 6: `internal/cli/snapshots_prune.go` `newPruneCommand` + `snapshots.go:14-79` attach as child — Use/Short/Long/Example, `--keep` `IntVar` read via `Changed()`, bound and no-args in `Args` (before the factory, so no home needed), no `--keep` → `config.DefaultKeep`, factory called with command `snapshots prune`; `internal/cli/snapshots_prune_test.go` `Test_prune_help_says_what_prune_keeps` (Long), `Test_prune_help_shows_examples_and_the_keep_flag` (Example, `--keep n` line), `Test_snapshots_help_lists_prune` (Short); `cmd/quarry/run_usage_test.go:146-149` rows `--keep -1`, `--keep abc` (cobra's actual text, captured), `prune extra`; `cmd/quarry/run_prune_refusals_test.go` `Test_run_snapshots_prune_without_keep_keeps_the_newest_12` (12 → none, 13 → one), `Test_run_snapshots_prune_names_itself_when_the_home_directory_cannot_be_resolved`
- [ ] Step 7: `internal/cli/render_prune.go` `renderPruned` (reuse `snapshotTaken` `render_snapshots.go:56-62`, `formatMB` `render.go:17`, `humanize.Count`), failure lines, `internal/cli/errors.go:5-12` `ReportedError` + `internal/cli/run.go:65-71` + `cmd/quarry/run.go:175-185` (exit 1, nothing more printed) — `render_prune_internal_test.go` `Test_renderPruned_names_what_it_kept` (N=1 `the newest one`, N≥2, store suffix, `1 snapshot`), `Test_renderPruned_aligns_the_rows` (`unknown` Taken, sizes of different widths, no trailing spaces, empty when nothing deleted); `run_prune_refusals_test.go` `Test_run_snapshots_prune_prints_one_line_per_failure_and_nothing_on_stdout_when_none_succeeded` (folder 0500: real EACCES, two lines), `Test_run_snapshots_prune_prints_what_it_deleted_before_the_interrupt_line`, `Test_run_snapshots_prune_says_it_was_interrupted_before_any_delete`, `Test_run_snapshots_prune_refuses_a_store_with_no_import_history`, `Test_run_snapshots_prune_refuses_a_store_of_another_format_naming_no_snapshot`, `Test_run_snapshots_prune_refuses_a_snapshots_folder_it_cannot_read`, `Test_run_snapshots_prune_protects_nothing_when_there_is_no_store`, `Test_run_snapshots_prune_reports_a_failed_stdout_write`, `Test_runWith_exits_1_and_prints_nothing_for_an_error_already_reported`

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`Prune`, `Pruned`, `WithRemove`, `ReportedError`); `internal/snapshot/doc.go:1-10` and `internal/cli/root.go:7-9` name prune

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2c-snapshots-config` → tick SCENARIO-21, 23, 25, 26, 27 with their acceptance tests (folds say "delivered by SCENARIO-21"); rewrite STATE.md; `status: done`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Three parts, one decision point each: `selectPrune` (pure, no I/O), the delete (pairs, then orphan sweep), and `(*Server).Prune` composing List → guards → select → delete — 24's `--dry-run` returns the selection and calls neither delete nor sweep; 29 runs select + delete over a listing it marks from the built snapshot's path (`list.go:57-74` + `markStoreSnapshot` `:170-178`), with no store read.
- `Pruned` carries facts, never lines: `Keep`, deleted `Entry`s, failed `Entry` + `osreason.Reason`, the store's `Entry` when kept outside the newest N, the count never attempted — 28's JSON, 29's warnings and its `sync interrupted while deleting…` line render from the same value; only `Prune` (the command's wrapper) attaches `snapshots prune interrupted…` and the cannot-tell refusal.
- Guards run before selection and before any remove: `keep < 1`, ended ctx, `Listing.StoreUnreadable != ""` — so `--dry-run` refuses identically (spec: "incl. dry run"); 24's Nothing-to-delete lines come after them, unless U7 rules the no-snapshots case the other way.
- Protection is `Entry.Store` alone (the `EvalSymlinks` match List already made), never a `StorePath` string compare — a store built `--from` a path outside the folder protects nothing (ruled row).
- Removal goes through `WithRemove` only, on paths built from the folder and a listed name; `Destination.Discard` stays Sync's (its fakes record `discardedPartial`).
- `cli.ReportedError` = "exit 1, already on stderr"; `runWith` prints nothing for it — 28 reuses it for JSON on a partial failure.
- Keep for 21 is `--keep` else `config.DefaultKeep`, chosen in `RunE`; 24 inserts `loadConfig("snapshots prune")` + `printConfigWarnings` at the top of `RunE` and swaps the fallback for `cfg.Keep`. The `Args` bound stays first, so `--keep 0` refuses even with a broken config.

**Left unbuilt** — named so nobody assumes it exists:
- `--dry-run` flag, `Would delete` block, both `Nothing to delete` lines, config load / `snapshots.keep` for prune — SCENARIO-24. Until then `--dry-run` is cobra's unknown-flag usage error (deletes nothing) although Long and Example name it, and a config `snapshots.keep` is ignored by prune.
- `prune --json` document — SCENARIO-28 (until then `--json` prints the text block). Auto-prune in `SyncAndImport`/`ImportFrom` — SCENARIO-29.
- U6, ruling owed before run B1: protection for a store whose recorded snapshot path no longer resolves. If ruled in, the fix belongs in `markStoreSnapshot` (`list.go:170-178`; it changes `snapshots` Status too), not in prune.

**Traps** — things that look right and are not:
- `os.Remove` deletes an empty directory: without the regular-file checks a directory named `<id>.sqlite` or `<id>.json` is removed.
- Sync commits the manifest before the snapshot (`snapshot.go:199-204`): an `<id>.json` beside `.<id>.sqlite.partial` is a sync in flight, not an orphan.
- `List` on an ended ctx returns no error when there is no store probe (`list.go:142-149`): `Prune` checks ctx itself.
- A snapshot with no manifest is normal; treating ENOENT on its `.json` as a failure makes every such snapshot "fail".
- A failed delete with nothing deleted and "nothing selected" both render empty stdout today; 24 must branch on the selection, not on `len(Deleted)`.
- A 0500 snapshots folder lists fine and fails every remove with EACCES: the real-shape fault for cmd tests; one-file faults need `WithRemove`.
