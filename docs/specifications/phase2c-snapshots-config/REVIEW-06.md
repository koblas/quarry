# Review Report — REVIEW-06

### Target
Fix pass 5, range `8da11fe..b04dc04`: one production line (`internal/snapshot/list.go` `snapshotName` strips any extension) plus one flipped table row and one cmd test.

### Verification (orchestrator)
- The change is the exact fix REVIEW-05's correctness-reviewer prescribed and trialled.
- Red first: the flipped row (`expected [20260929T090011Z] actual []`) and the new cmd test (`Deleted 1 snapshot …`, X's `.sqlite`/`.json` gone) failed before the change.
- Mutation: restoring the `.sqlite`-only compare reddens both, on any volume.
- `go test rc=0` (one covered full suite); `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 8da11fe46ef3`; `-race` on internal/snapshot and cmd/quarry ok; `golangci-lint run ./...` → `0 issues.`, rc=0; `spec-check.py --run` OK. test-stats cmd/quarry 277 (+1).

### Skipped reviewers
- correctness-reviewer: not re-run for a one-line change that is its own prescribed and trialled fix, with red evidence and a killing mutation. Five correctness rounds have now examined `markStoreSnapshot`; the safety guarantee (every same-file entry spared) is by construction and name-independent, and arm 3 now follows the spec text exactly.

### Open (deferred to STATE.md `## Open debts`, final product-vision pass for the copy items)
- Unreadable recorded path (non-ENOENT stat fault) treated as gone → nothing marked; proposed cannot-tell refusal, reason phrase owed.
- A spared same-file entry is kept silently (not named in the keep phrase).
- `store_snapshot.id` in `--json` from `ID(recorded)` can differ from the marked entry's id.
- Unknown-key warning does not re-quote key parts.
- Missing lock between prune and a concurrent sync; last-writer-wins history under concurrent syncs.

### Verdict: PASS WITH FOLLOW-UPS
