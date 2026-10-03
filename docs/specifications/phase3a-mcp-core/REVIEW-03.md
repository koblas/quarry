# Review Report — phase3a-mcp-core, gate round 3 (after fix pass 2, f7f8af9: per-open DuckDB instance cache)

### Target
8ea913c..HEAD

### Triggered reviewers
- correctness-reviewer, arch-reviewer (driver global override), test-reviewer

Mutation sample: 6 of 6 — 5 killed, 0 survived, 1 non-viable; 102s. Coverage: 0 uncovered added lines.

### BLOCKER
none

### MAJOR
- correctness-reviewer: internal/platform/duckdb/cache.go:27-32 — hook reads `openCache` without `openMu`; parallel duckstore tests open DuckDB raw (`internal/store/duckstore/status_test.go:69,105,133`, `history_test.go:144` via parallel rates tests, `history_faults_test.go:380`) while parallel tests run `openDB`. A raw open can take another open's private cache, which that open then destroys → C use-after-free / data race in the test binary. Production has no raw duckdb opens. Fix: route those sites through an exported opener in internal/platform/duckdb (e.g. `OpenReadWrite(ctx, path)` → `openDB(path, nil)`); make `openCache` an `atomic.Pointer`.
- test-reviewer: internal/platform/duckdb/duckdb.go:216 (and :92, ping-failure `release`) — `mapping.DestroyInstanceCache` unpinned; replacing it with `_ = d.cache` stays green. MCP opens per call → unbounded C cache leak undetected. Fix: assert `earlier.cache.Ptr == nil` after Close (+ second Close idempotent); state failed-open arms as unobservable in default build or pin otherwise.

### MINOR
- test-reviewer: internal/platform/duckdb/cache.go:39 — `openMu` pinned only under -race; add deterministic white-box test (hold lock → open does not return; unlock → returns within bound).
- correctness-reviewer: internal/platform/duckdb/duckdb.go:49 — `Create` still uses the shared cache under `openMu` (ctx-blind); give it a private cache too, destroy in release.
- correctness-reviewer: internal/platform/duckdb/duckdb.go:85-103 — N concurrent calls → N full instances with default threads/memory_limit; consider explicit limits in readOnlyDSN (policy — deferred unless ruled).
- correctness-reviewer: duckdb.go:213-219 / STATE trap — stranded instances accumulate threads, buffer pool and an open fd on a replaced store until exit (documented trade-off).
- arch-reviewer: cache.go:19-23 — openDB-only constraint unenforced mechanically; proposed depguard/forbidigo rule in .golangci.yaml (user decision; not edited).

### NIT
- arch-reviewer: cache.go:18 `sharedCache` package var → local in init; cache.go:3-13 note that the strand test is the canary on driver upgrades.
- test-reviewer: open_instance_internal_test.go:14-31 second act in Then; duplicate store helpers `writeStoreHolding` / `newStoreFile`.

### Verdict: BLOCKED
