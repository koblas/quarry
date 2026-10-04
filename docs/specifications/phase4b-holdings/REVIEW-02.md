# Review Report — phase4b-holdings, round 2 (re-gate)

### Target
Fix pass `cb64f3d..HEAD` (findings from `REVIEW-01.md`).

### Triggered reviewers
- test-reviewer: blocked in round 1.
- correctness-reviewer: the fix touched production logic (`holdingWalker.started`, `report.ResolveAsOf`). The first spawn failed on an API spend-limit error; the retry ran.

### Skipped reviewers
- arch-reviewer, refactor-advisor: round 1 was PASS WITH FOLLOW-UPS. The fix's production edits are a shared helper (`ResolveAsOf`, which was their own suggestion), constant spellings and a doc comment, with no import or placement change.

### Gate inputs
- uncovered-diff vs cb64f3d: 0 added lines.
- test-stats (base cb64f3d): TOTAL 1882 (+10).
- Mutation sample, first run: aborted with "unmutated tests fail for internal/store/duckstore" in its isolated copy. Running `go test -count=1 ./internal/store/duckstore/` in the worktree passed.
- Mutation sample, re-run: `6 sampled of 6 candidates since cb64f3d — 6 killed, 0 survived, 0 non-viable, 0 timed out; 729s`.

### Round-1 findings
- MAJOR 1, MCP holdings deadline row: closed.
- MAJOR 2, `--all` pin: closed.
- MAJOR 3, zero and placeholder price through the real reader, plus the cmd zero-price test: closed.
- MINOR folds: `account_filter` JSONEq, text-is-raw, older-format refusal, MCP `accounts: []` on a populated day: closed.
- correctness MINOR, the `started` flag: closed.
- arch/refactor MINOR, `ResolveAsOf`: done. NIT `Convertible` money constants: done. `Actions` doc: done.
- Rest: deferred in STATE.md ("Gate round 1 deferred").

### New findings
None.

### Verdict: PASS
