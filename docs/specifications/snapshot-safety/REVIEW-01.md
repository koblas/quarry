## Review Report — round 01 (2026-10-07)

### Target
Changed files 047e3f33..937c8d64 (merge-base with origin/main → HEAD).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: production Go under cmd/ and internal/
- test-reviewer: `*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` changes

### Gate inputs
- verify.sh 047e3f33: build, suite, race, lint all rc=0; uncovered-diff 0 open, 1 declared unreachable (internal/snapshot/from.go:110 — found reachable, see MAJOR 1).
- spec-check --run snapshot-safety: OK.
- mutation-sample: 20 of 72 — 18 killed, 0 survived, 2 timed out. test-reviewer re-ran both: destination.go:112 killed in ~9 s; destination.go:94 SURVIVED (MAJOR 2).

### BLOCKER
none

### MAJOR
1. correctness-reviewer — internal/snapshot/from.go:107-111: the `// unreachable:` claim is false on darwin. `filepath.Abs` → `os.Getwd` fails when the working directory has no search permission, so `quarry sync --from x.sqlite` prints the unruled `quarry: resolve snapshot path x.sqlite: stat .: permission denied`, exit 1. **Ruled as F7** (spec "--from resolution" table): `quarry: cannot resolve x.sqlite against the current folder: <OS reason per G1>; pass --from an absolute or ~/ path instead`, value shown as given, `causedRefusalError`, exit 1, stdout empty under `--json`; `osreason.Reason` also unwraps `*os.SyscallError` (Linux `getwd: …`). Fix (test-first): darwin-runnable tests at slice and cmd level (cwd chmod 0 after `t.Chdir`, restore in Cleanup), text and `--json`; success controls `--from /abs/x.sqlite` and `--from ~/x.sqlite` from the same folder; an `osreason.Reason` row for a constructed `*os.SyscallError`; re-point `Test_import_from_refuses_a_relative_path_when_the_working_directory_no_longer_exists` (import_from_test.go:154) to the F7 bytes; delete the `// unreachable:` marker.
2. test-reviewer — internal/snapshot/destination.go:94 (`Backup`): the stat-check fallback `fileExists(snapshotFinal) || fileExists(manifestFinal)` is unpinned; mutant `||`→`&&` survives the whole suite. With a healthy listing `folderUses` already sees any final, so the stats matter only on a listing fault — the BR-C5 fallback — and then the mutant reuses the name and `atomicfile.Commit` renames over an existing snapshot. Fix (pin-only): a test with `NewDirDestinationReading` whose readDir returns `fs.ErrPermission` over a temp dir seeded with (a) `<id>.sqlite` only and (b) `<id>.json` only, each expecting `<id>_2`. Mutations: `||`→`&&`; delete each disjunct → red.

### MINOR
- correctness — internal/snapshot/list.go:67 `StoreWarningAbsolute` doc overclaims "any path absolute"; the R3 store-fault arm keeps the `~` form (spec :9 out of scope). → FOLD: reword.
- correctness — internal/snapshot/lock.go:40-46 prune proceeds unlocked when the quarry folder is missing (no constructible delete). → FOLD: one-line why comment.
- correctness — internal/snapshot/destination.go:108-116 BR-C5 fail-open on a listing fault (ruled, SCENARIO-13 ruling 4). → no change; already an accepted gap.
- arch — os.Stat/os.SameFile not behind a seam (list.go:266,311, from.go:126). → Open debt.
- refactor — internal/snapshot/select.go `selectFolder` ~55 lines, three phases (compose method). → Open debt.
- refactor — internal/snapshot/auto_prune.go:27 silent skip on cannot-tell. → Rejected: BR-U4 rules no new warning.
- refactor — internal/snapshot/list.go:588-596 `markStore` builds abbreviated/absolute reason by hand; parallel abbreviated/absolute field pairs. → Open debt.
- refactor + arch — internal/report/report.go:62 vs internal/snapshot/import.go `ID`: "strip .sqlite in any case" implemented twice. → Open debt.
- refactor — the "extension in any letter case" fact restated on ~9 symbols (destination.go:28-31, import.go, from.go, select.go, report.go). → FOLD: keep it on `sqliteExtension` and the two patterns (and `report.snapshotExt`); callers drop the restatement.
- refactor — doc budgets: list.go `Entry` (5 lines; dup of folderSelection's fact), auto_prune.go:11-13 (3 lines), lock.go `LockForSync`/`LockForPrune` repeat what `lock()` enforces, ports.go `Locker` and doc.go:11-13 restate the lock sentence, list.go `StoreUnreadable` 150+ chars. → FOLD.
- test — cmd/quarry/run_sync_lock_test.go:169 `if c.withBundle` in loop → FOLD (arrange func field).
- test — internal/snapshot/destination_letter_case_test.go:61-70 readDir closure with `if` → FOLD (named helper).
- test — cmd/quarry/run_prune_recorded_cells_test.go:58-76 dry-run cells assert only two ids via Contains → FOLD (exact stdout / `jsonIDs` would_delete incl. store snapshot absent).
- test — cmd/quarry/run_prune_recorded_test.go:20 `recordedClass.marked` unread → FOLD (delete).
- test — cmd/quarry/run_prune_recorded_test.go:99-130 test + helper duplicate a cells row → FOLD (delete).
- test — cmd/quarry/run_prune_lock_test.go:207-224 text cell Contains → FOLD (exact stdout).
- test — cmd/quarry/run_sync_lock_test.go:241 name over-narrow → FOLD (rename "reads while a sync holds the lock").
- test — internal/cli/snapshots_prune_lock_test.go:138-146 duplicates the release event → FOLD (delete).
- test — internal/snapshot/from_internal_test.go:48-100 white-box locateFrom rows duplicate black-box pins → Open debt (keep isPathForm table).

### NIT
- arch — lock_refusal.go type-switches on *lockfile.Error; refactor — lockfile Error() omits e.Err, two-switch lock_refusal, snapshots_prune.go prune-func reassign order; test — fd reuse in flock test, fifo goroutine leak on timeout, no N-way Acquire race. → Open debts / no action.
- product-vision (F7 ruling) — internal/snapshot/bundle.go:98-101 `ResolveBundlePath` (`sync --quicken <relative>`) has the same unresolvable-cwd bare error and unreachable comment; pre-existing, outside this feature. → Open debt (needs its own copy ruling).

### Strengths
- `selectFolder` pure table tests over fake `fs.DirEntry` run case-sensitive-volume rows on APFS; `WithReadDir` wraps real entries.
- Re-exec kill -9 holder and deadline-bounded refusal tests for flock.
- Lock taken after usage/config and before any write in every writer path (LSP-verified); destructive guards hold under every recorded-path class.

### Verdict: BLOCKED (2 MAJOR)
