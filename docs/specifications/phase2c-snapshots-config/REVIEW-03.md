# Review Report — REVIEW-03

### Target
Re-gate after fix pass 2, range `22b5da7..242ad35`.

Coverage gate: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 22b5da7eb339`. `spec-check.py --run`: OK. Mutation sample (fix range): 5 sampled of 5 — 5 killed, 0 survived; 96s.

### Triggered reviewers (narrow, by concern)
- correctness-reviewer: `internal/snapshot/list.go`, `auto_prune.go` changed.
- test-reviewer: tests changed; its REVIEW-02 MAJOR re-checked.

### Prior findings
- REVIEW-02 MAJOR 1 (two entries marked): closed for a recorded path inside the folder. REVIEW-02 MAJOR 2 (`valueText` unpinned): closed; the line-152 tab-arm mutant is equivalent (`strings.Fields`). REVIEW-01 BLOCKER (case-variant `--from`): still closed. `newestFirst` doc MINOR: closed. `// unreachable:` at `auto_prune.go:46`: holds.

### BLOCKER
1. correctness-reviewer — `internal/snapshot/list.go:225-231` `storeEntryIndex`: when the store recorded a symlink and a newer hard link of its target sits in the folder, the "newest SameFile match" arm marks the hard link, so prune deletes the store's real snapshot, then the link. Introduced by fix pass 2 (the rule REVIEW-02 prescribed).
   Failure (reproduced at cmd level): `quarry sync` takes X; `ln -s snapshots/X.sqlite ~/latest.sqlite`, `cp snapshots/X.json ~/latest.json`, `quarry sync --from ~/latest.sqlite` (exit 0; `resolveFrom` `from.go:83` takes `filepath.Abs` with no symlink resolution, so the store records `~/latest.sqlite`); `ln snapshots/X.sqlite snapshots/29980101T000000Z.sqlite` (Y, newer than X); one more newer snapshot exists. `snapshots --json`: `"store": true` on Y, false on X. `snapshots prune --keep 1` run 1, exit 0: `Deleted 1 snapshot (1.5 MB), keeping the newest one and 29980101T000000Z, the store's snapshot:` — X.sqlite and X.json gone, `~/latest.sqlite` dangles, `sync --from 29980101T000000Z` exits 1 (no manifest). Run 2 deletes Y: the last copy of the store's snapshot is gone. Base 22b5da7: `Nothing to delete`. Second way in: `ln -s X.sqlite snapshots/20000101T000000Z.sqlite` + copied manifest, `sync --from 20000101T000000Z` (`fromPathRefusal` follows the link; `scanFolder` does not list a symlink). Auto-prune shares the function.
   Fix: among the SameFile matches also accept the entry whose ID `EqualFold`s `ID(filepath.EvalSymlinks(recorded))`, before falling back to the newest match (trialled: both probes keep X across two prunes; snapshot and cmd packages green).
   Orchestrator note: third pass on this one function. The invariant to pin directly: **prune never deletes the folder entry the recorded path resolves to.**

### MAJOR
none

### NIT
- test-reviewer — `cmd/quarry/run_store_link_test.go:70-82` JSON unmarshalled twice; `internal/snapshot/auto_prune.go:46` unreachable comment cites `list.go` line numbers (name `scanFolder` instead); `internal/config/parse.go:152` tab arm is dead (equivalent mutant) — drop or keep for symmetry.

### Could not check
- A case-sensitive volume; one device/inode for distinct files; the auto-prune route to the BLOCKER at cmd level; a real SIGINT.

### Verdict: BLOCKED
1 BLOCKER (introduced by fix pass 2), 0 MAJOR, 3 NIT. test PASS WITH FOLLOW-UPS; correctness BLOCKED.
