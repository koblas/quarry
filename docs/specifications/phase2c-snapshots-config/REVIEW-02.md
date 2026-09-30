# Review Report — REVIEW-02

### Target
Re-gate after fix pass 1, range `b8ddfce..81db942`.

Coverage gate: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since b8ddfcef6c08`. `spec-check.py --run`: OK. test-stats TOTAL +21. Mutation sample (fix range): 15 sampled of 15 — 13 killed, 2 survived (`internal/config/parse.go:152` real gap, below; `parse.go:206` equivalent — `present` is never read when `err` is set).

### Triggered reviewers (narrow, by concern)
- correctness-reviewer: production logic changed (`internal/snapshot/list.go`, `prune.go`, `auto_prune.go`, `import.go`; `internal/config/parse.go` rewritten; `duckstore/history.go`; `store/open.go`).
- test-reviewer: tests changed; its MAJOR re-checked.

### Skipped reviewers
- arch-reviewer: no import, placement or wiring change. refactor-advisor: no finding of its own claimed closed. api-reviewer, pipeline-reviewer: no trigger.

### Prior findings
REVIEW-01 BLOCKER 1 and MAJORs 1–4: all **closed** (both reviewers, with mutation evidence). Folded MINORs (prune sweep after ctx ended, `import.go` evaluation order, causeless error, empty Path, inline table raw, `UnreadableReason` table, ENOTDIR, cmd assertion fixes): closed.

### BLOCKER
none

### MAJOR
1. correctness-reviewer — `internal/snapshot/list.go:205-211` `markStoreSnapshot`: the `os.SameFile` pass marks every entry sharing the recorded file's identity; everything downstream assumes exactly one store entry. Introduced by fix pass 1 (df68d89).
   Failure (reproduced at cmd level): five snapshots, store built from `20260927T143005Z`, `ln snapshots/20260927T143005Z.sqlite snapshots/20200101T000000Z.sqlite`. `quarry snapshots`: two rows read `store`; `--json`: two entries `"store": true` while `store_snapshot.id` is `20260927T143005Z`; `snapshots prune --keep 1 --dry-run`: `Would delete 3 snapshots (2.6 MB), keeping the newest one and 20200101T000000Z, the store's snapshot:` (names the link; base said `Would delete 4 … and 20260927T143005Z`). `selectPrune` (`prune.go:44-55`) holds one `storeKept`, overwritten by each marked entry. Nothing is deleted wrongly.
   Fix: mark exactly one entry. Among the SameFile matches take the one whose ID `strings.EqualFold`s `ID(recorded)`; if none does, take the first (newest) match; then the existing ID fallback. Do NOT use plain "first SameFile match wins" (with recorded X and a newer link Y it marks Y, prune deletes X, the next run's stat of the recorded path fails and Y becomes prunable). Regression tests: (a) a link inside the folder under a second valid ID → only the recorded ID is marked; (b) cmd-level prune line names the recorded ID and the delete count includes the link. The three tests in `list_store_identity_test.go` stay green.
2. test-reviewer — `internal/config/config_test.go` (got-value tables ~190-200, ~216-223): no test writes `=` without a space on both sides, so `valueText` (`parse.go:143-157`) is unpinned — mutation survivor at `parse.go:152`.
   Failure: mutant `(f.data[start] != ' ' || …)` panics on `[snapshots]\nkeep=0\n` and `keep =0` (`slice bounds out of range [19:18]`) with every test green; spaced rows kill nothing (`strings.Fields` trims).
   Fix: add rows to `Test_load_refuses_a_snapshots_keep_below_one_or_not_an_integer` and the quicken path table: `keep=0`, `keep =0`, `keep= 0`, `snapshots.keep\t=\t0`, `keep=0# none` → `got 0` each. Production already handles them.

### MINOR
- correctness-reviewer — `internal/snapshot/list.go:156-157` doc says equal-value suffixes fall to "fewer zeros first" but all-zero suffixes sort the other way (`_000, _00, _0, (none)`); P2c-5 does not rule equal-value order — correct the doc sentence.
- correctness-reviewer — `internal/config/parse.go:247` unknown-key warnings join key parts with `.` without re-quoting (`"snapshots.keep" = 5` → `unknown key snapshots.keep`; `"a\nb" = 1` breaks the warning across two lines; `"" = 5` → `unknown key ;`). Pre-existing; needs a product-vision copy ruling. → final pass.

### NIT
- correctness-reviewer — a comment inside a multi-line value is part of the raw `got` text (literal under C2m); a UTF-8 BOM is refused with go-toml's message (C1 form).
- test-reviewer — `unlistedReason`'s no-cause fallback is reachable only white-box; `skipOnCaseSensitiveVolume` duplicated in two packages.

### Could not check
- A real case-sensitive volume (four tests skip there; the two non-skipping identity tests pin both arms); a filesystem reporting one device/inode for distinct files; a real SIGINT.

### Strengths
- 152 inputs diffed through `config.Load` at both commits: every ruled line byte-identical; a 46-second fuzz of the parser walk found no panic, empty `got` or newline in a refusal.
- Each identity arm pinned by a test that runs on any volume (hard link → SameFile; unresolvable recorded path → EqualFold).

### Verdict: BLOCKED
0 BLOCKER, 2 MAJOR (one introduced by fix pass 1), 2 MINOR, 4 NIT.
