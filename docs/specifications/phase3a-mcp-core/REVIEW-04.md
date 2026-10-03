# Review Report — phase3a-mcp-core, gate round 4 (after fix pass 3, c39ced4)

### Target
1df76c1d2ed0a0a8ddd0ec1c3fccca7be529b933..HEAD

### Triggered reviewers
- correctness-reviewer, test-reviewer (both blocked in round 3)

Mutation sample: 5 of 5 — 5 killed, 0 survived; 279s. Coverage: 0 uncovered added lines. SCENARIO-17: 50/50 serial passes.

### BLOCKER / MAJOR
none (round-3 MAJORs confirmed closed: raw-open race — OpenReadWrite + atomic openCache, six fixtures rerouted; DestroyInstanceCache pinned on every path)

### MINOR
- correctness-reviewer: internal/platform/duckdb/duckdb.go:229-233 — release idempotent only sequentially; concurrent Close of one DB would double-destroy; no caller does — doc "on the same goroutine" or sync.Once.
- test-reviewer: internal/platform/duckdb/open_instance_internal_test.go:1-6 — header grew to 6 lines; canary note duplicated with cache.go:15-18.
- test-reviewer: Test_close_destroys_the_instance_cache_and_a_second_close_is_safe — two behaviours in one test.
- test-reviewer: internal/platform/duckdb/cache.go:15-18 — production comment names a test func.

### Unchecked
- DuckDB C++ lifetime when a private cache is destroyed while a stranded instance lives (no C++ source available; ordering exercised by a passing test only).

### Verdict: PASS WITH FOLLOW-UPS
