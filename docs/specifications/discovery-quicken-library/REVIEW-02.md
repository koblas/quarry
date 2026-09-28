# Review Report — round 02 (re-gate)

### Target
Range `cc1a9a3..b0f6dc6`. Coverage: 0 uncovered.

### Triggered reviewers
correctness (owner of the REVIEW-01 MAJOR). Test/refactor MINORs closed in the same pass.

### Status
REVIEW-01 MAJOR closed: ENOTDIR on the Quicken location is missing (both ancestor and folder-itself cases tested); `~/Documents` keeps R3 (pinned). Mutations M1, M2, M4, M5 each redden their named test.

### MINOR (deferred — STATE.md Open debts)
- `discover.go:95-99` per-entry stat returning ENOTDIR (symlink through a regular file) is refused rather than skipped as dangling. Pre-existing per-entry filter; rare.

### Verdict: PASS WITH FOLLOW-UPS
