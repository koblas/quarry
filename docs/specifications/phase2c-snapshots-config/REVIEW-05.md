# Review Report — REVIEW-05

### Target
Re-gate after fix pass 4, range `94165347..9da76bc`.

Gate output on HEAD: `go test rc=0`; `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 94165347f014`; lint 0 issues; `spec-check.py --run` OK. Mutation sample: 3 sampled of 3 — 3 killed; 72s.

### Triggered reviewers
- correctness-reviewer (narrow): `internal/snapshot/list.go`, `prune.go`.

### Prior findings
- REVIEW-04 BLOCKER (extension-case variant): **closed** on all five routes (symlink to `X.SQLITE` over three prunes; recorded `<dir>/X.SQLITE`; real `sync --from ~/latest.sqlite`; auto-prune with 12 newer snapshots; in-folder alias and direct `--from`). REVIEW-03, REVIEW-02, REVIEW-01 probes: still closed. Same listing, within-cap path, orphan sweep, counts/copy/JSON under sparing: checked and clean.

### BLOCKER
1. correctness-reviewer — `internal/snapshot/list.go:239-245` (arm 3 at `:235`): `snapshotName` strips only a `.sqlite` extension, so when the recorded path is gone and was named `<ID>.<other ext>`, arm 3 marks nothing and prune deletes the store's snapshot; spec P2c-6 and U6 say "the recorded path's base name without extension". Pre-dates the range, but the in-range row "a path that is gone named as a snapshot under another extension marks nothing" pins the wrong behaviour.
   Failure: `quarry sync` takes X; `ln <snapshots>/X.sqlite ~/backup/X.db`, `ln <snapshots>/X.json ~/backup/X.db.json`; `quarry sync --from ~/backup/X.db` exit 0; `rm ~/backup/X.db ~/backup/X.db.json`; `snapshots prune --keep 1` → `Deleted 1 snapshot (1.5 MB), keeping the newest one:`, X.sqlite and X.json gone. Controls with `~/backup/X.sqlite` / `X.SQLITE` keep X.
   Fix: `snapshotName` strips any extension (`strings.TrimSuffix(name, filepath.Ext(name))`); snapshot IDs contain no dot; over-marking only keeps more. Flip the row to `want: []string{idMiddle}`; mutation: restoring the `.sqlite`-only compare reddens it on any volume.

### Could not check
- A case-sensitive volume; Linux's symlink hop limit; one device/inode for distinct files; a real SIGINT.

### Verdict: BLOCKED
1 BLOCKER (arm 3 name rule), 0 MAJOR.
