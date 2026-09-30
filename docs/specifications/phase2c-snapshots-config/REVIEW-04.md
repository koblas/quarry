# Review Report — REVIEW-04

### Target
Re-gate after fix pass 3, range `94d8e03..4236a9e`.

Gate output on HEAD: `go test rc=0`; `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 94d8e03e7c79`; lint 0 issues; `spec-check.py --run` OK. Mutation sample: 2 sampled of 2 — 2 killed; 40s.

### Triggered reviewers (narrow, by concern)
- correctness-reviewer: `internal/snapshot/list.go` decision consolidated (`markStoreSnapshot` → `storeEntryIndex` → `entriesAt`).

### Prior findings
- REVIEW-03 BLOCKER (symlink-recorded store snapshot): closed for a correctly-cased target (both probes, the real `sync --from ~/latest.sqlite` sequence, the in-folder alias, and the auto-prune route run for real); **open** for the extension-case variant below. REVIEW-01 case-variant BLOCKER and REVIEW-02 hard-link MAJOR: stay closed. Arm-3 gating ("no entry is the recorded file") conforms to U6.

### BLOCKER
1. correctness-reviewer — `internal/snapshot/list.go:217-218` (and `:228`): arm 1 compares `ID(EvalSymlinks(recorded))` / `ID(recorded)`, and `ID` (`import.go:89-91`) strips only a lowercase `.sqlite`; a path that reaches the entry as `X.SQLITE` misses arm 1 and arm 2 marks the newer hard link.
   Failure (real commands, case-insensitive volume): `quarry sync` takes X; `ln -s <snapshots>/X.SQLITE ~/latest.sqlite`; X.json linked as `~/latest.json`; `quarry sync --from ~/latest.sqlite` → exit 0; `ln <snapshots>/X.sqlite <snapshots>/29980101T000000Z.sqlite` (Y); one newer snapshot exists. `snapshots --json`: marked=[Y]. Prune 1: `Deleted 1 snapshot (1.5 MB), keeping the newest one and 29980101T000000Z, the store's snapshot:` → X.sqlite and X.json gone. Prune 2 deletes Y. Auto-prune route (12 newer snapshots, one real `sync --from ~/latest.sqlite`): `Pruned    1 snapshot beyond the newest 12 and the store's own` → X gone. In-folder alias (`ln -s X.SQLITE <snapshots>/20000101T000000Z.sqlite`) and direct `sync --from <snapshots>/X.SQLITE` (manifest at `X.SQLITE.json`): same.
   Fix (reviewer): derive the names in `storeEntryIndex` with the extension removed in any letter case.
   **Orchestrator ruling (fourth variant of one defect — change the design, not the name rule):** safety must not depend on choosing the right single entry by name. Prune and auto-prune spare EVERY folder entry that is the same file (`os.SameFile`) as the recorded path; name matching only picks which one entry carries the `store` mark for display. Plus the reviewer's extension-case fix so the display mark lands on X.

### MINOR
- `internal/snapshot/list.go:235` — a recorded path that cannot be read (e.g. a symlink under a `chmod 000` folder, or a chain the kernel refuses) is treated as gone: nothing marked, the store's snapshot prunable. Conforms to U6b; ruling owed (route a non-ENOENT stat fault to the cannot-tell refusal; needs a reason phrase) → final product-vision pass.
- `internal/snapshot/list.go:223-226` — arm 2 keeps the newest same-file entry even when it has no manifest and an older same-file entry does (no constructible failure; superseded by the ruling above: all same-file entries are spared).

### NIT
- `internal/snapshot/list.go:217` — the dropped `EvalSymlinks` error has no reason at the site.

### Could not check
- A case-sensitive volume (the BLOCKER cannot occur there); Linux's symlink hop limit; one device/inode for distinct files; a real SIGINT.

### Verdict: BLOCKED
1 BLOCKER (variant of REVIEW-03's), 0 MAJOR, 2 MINOR, 1 NIT. Fourth fix pass opens (BLOCKER).
