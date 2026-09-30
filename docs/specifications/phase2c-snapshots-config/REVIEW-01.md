# Review Report — REVIEW-01

### Target
Feature branch `7dd4f06..e06aa41` (phase2c-snapshots-config), all changed Go files.

Coverage gate: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 7dd4f0656f51; 4 declared unreachable` (`internal/cli/output.go:38`, `snapshots.go:65`, `snapshots_prune.go:88`, `internal/config/parse.go:99` — all four judged to hold by correctness-reviewer). `spec-check.py --run`: OK. test-stats TOTAL 1038 (+314). Mutation sample: 20 sampled of 162 — 20 killed, 0 survived; 858s.

### Triggered reviewers
- arch-reviewer, correctness-reviewer (×2, split by package), refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer (×2: cmd/quarry, internal/**): `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` change

### BLOCKER
1. correctness-reviewer — `internal/snapshot/list.go:202-215` `markStoreSnapshot` compares path and ID case-sensitively; a store built with a case-variant `--from` value is not recognised and its own snapshot is deleted.
   Failure (reproduced on default case-insensitive APFS): `quarry sync --from 20260930t134840z` resolves (`from.go:78` joins the value as typed; `os.Stat` succeeds) and records the lowercase path; `EvalSymlinks` keeps the input's case so the path arm misses; `ID()` gives the lowercase id so the ID arm misses. Auto-prune with 12 newer snapshots: exit 0, `Pruned    1 snapshot beyond the newest 12`, the store's own `.sqlite` and `.json` gone. Manual: `quarry snapshots` marks no row `store`; `snapshots prune --keep 1` deletes it.
   Fix: decide identity with `os.SameFile` on `os.Stat` of the recorded path and each entry path (not string equality of resolved paths); make the ID fallback `strings.EqualFold`. Regression test: cmd-level `sync --from strings.ToLower(id)` with the store's snapshot beyond the cap, asserting the file survives; plus `snapshots` marks it and `prune` protects it.

### MAJOR
1. correctness-reviewer + test-reviewer — `internal/store/duckstore/duckstore.go:550` `importRunRows` numbers the new run `maxID + i + 1` with no overflow guard; spec CF2 row (`its import_runs table has an id too large to follow`) has no production code and no pin.
   Failure: previous store with `import_runs` id 9223372036854775807 → next sync succeeds with no HistoryFault and appends id -9223372036854775808; `Status`/`BuiltFrom` (`ORDER BY id DESC LIMIT 1`) then report the OLD run, so `status` shows the wrong build, `snapshots` marks and `prune` protects the wrong snapshot; the sync after that fails on the duplicate key.
   Fix: in `readRuns` (`history.go:119-135`) treat `maxID == math.MaxInt64` as a history fault with that reason → CF2, history restarts at id 1. Tests: id MaxInt64 → reason phrase + ids `[1]`; control id MaxInt64-1 carries and numbers the new run MaxInt64.
2. correctness-reviewer — `internal/config/parse.go:222` `unknownKeys` decodes into a tagged struct (go-toml matches case-insensitively) while the value lookup (`parse.go:140-148`) is exact: a case-variant key is neither applied nor warned about.
   Failure: `[Snapshots]\nKeep = 50`, 20 snapshots, `quarry sync`: exit 0, stderr empty, `Pruned    9 snapshots beyond the newest 12`. Same for `SNAPSHOTS.KEEP = 50`; `[Quicken]\nPath = 12` gives no warning and no C2q (falls through to discovery).
   Fix: identify unknown keys by exact name — walk the parsed expressions in file order and report every key path that is not exactly `snapshots`, `snapshots.keep`, `quicken`, `quicken.path` (C3: `unknown key Snapshots`).
3. correctness-reviewer — `internal/config/parse.go:190-202` `rawKeep`/`rawPath` read the raw value through the same case-insensitive struct decode, so a refusal can quote a different key's value.
   Failure: `[snapshots]\nkeep = "x"\nKeep = 9` prints `… got 9`; `[quicken]\npath = 12\nPATH = "~/ok.quicken"` prints `… got "~/ok.quicken"`.
   Fix: read raw values from exact-keyed maps (`map[string]map[string]unstable.RawMessage`), not tagged structs.
4. correctness-reviewer — `internal/snapshot/list.go:157-163` `newestFirst` orders the `_N` suffix by length then text, not numeric when a suffix has leading zeros (`snapshotFilePattern` admits them; P2c-5 says numeric).
   Failure: `_010` (10) sorts newer than `_11` (11); straddling the cap, prune deletes `_11` and keeps `_010`. Only hand-named files reach it.
   Fix: strip leading zeros from both suffixes before the length and text compares.

### MINOR
- correctness-reviewer — `internal/snapshot/prune.go:131` `Prune` sweeps orphan manifests without checking ctx, unlike `autoPrune` (`auto_prune.go:39`); test-reviewer: `prune_sweep_test.go:193` and `auto_prune_test.go:412` pin opposite outcomes. Orchestrator ruling: one rule for both (the auto-prune ruling) — skip the sweep once ctx has ended; align the prune test and STATE.
- correctness-reviewer — `internal/snapshot/import.go:136` `return outcome, s.autoPrune(ctx, &outcome)` reads and mutates `outcome` in one return (order unspecified). Fix: `err := s.autoPrune(ctx, &outcome); return outcome, err`.
- correctness-reviewer — `internal/snapshot/auto_prune.go:23` `osreason.Reason(errors.Unwrap(err))` panics on a causeless error; fall back to `Reason(err)`.
- correctness-reviewer — `internal/config/parse.go:156-158` `got()` names an inline table (`keep = { a = 1 }`, written right of `=`) `a table`; C2l rules raw collapsed text for values right of `=`. Fix: `gotTable` only when a `[key]` header introduced the value.
- correctness-reviewer — `internal/store/open.go:54` `ReplaceAll(e.Reason, e.Path, at)` with empty Path inserts `at` between every rune; guard `if e.Path == "" { return e.Reason }`.
- correctness-reviewer — `internal/cli/snapshots_prune.go:91`, `sync.go:137`: a stdout-write failure returns before the stderr lines still owed (matches the existing `emit` idiom; not required).
- arch-reviewer — `internal/snapshot/snapshot.go:95` `WithRemove` is a second delete seam beside `Destination.Discard`; `list.go:120`/`prune.go:156` read the filesystem directly (follows `discover.go`); `internal/config/config.go:32` `os.ReadFile` direct; `internal/store/open.go:41-53` user-facing phrases in the port package.
- test-reviewer (cmd) — shared helpers and three ID-constant families live in per-scenario files; duplicated helpers (folder listing ×3, snapshots dir ×3, corrupt-store setup ×15, five-fixture list ×3); `run_prune_json_test.go:302,332,336,497,505` `if` in test bodies and a mixed-behaviour refusal table; `run_snapshots_json_test.go:337` loop asserts with no `require.Len` (vacuous on empty); weak `Contains` at `run_prune_keep_config_test.go:130`, `run_prune_refusals_test.go:262`, `run_sync_prune_interrupt_test.go:51`; overlapping config matrices ×3 commands and nested `warnings[]` prefix tests; implicit Given values (`verified` zero value, zero `taken` = no manifest); `run_prune_dryrun_test.go:166` name claims an ordering it cannot show; sync prune tests use `megabytes()` instead of literals; file grouping/size; naming (`requireSnapshots*` call `assert`; "honours").
- test-reviewer (internal) — `auto_prune_test.go:246` schema-mismatch case `want: nil` asserts only `require.Error`; `auto_prune_test.go:330-369` table with two assertion shapes and `if … return`; `if` in table loops (`prune_store_test.go:44,100,152`, `prune_dryrun_test.go:131`); duplicated fakes/builders across snapshot tests (`failing` vs `failingRemover`, `entryIDs` vs `doomedIDs`, three folder builders, `snapshotIDFromPath` vs `snapshot.ID`); duplicated duckstore fixtures (unopenable-store builders, three import_runs DDL builders); `errStoreRead` fakes a `BuiltFrom` error shape the adapter never returns; mirrored `Prune`/`PlanPrune` test pairs; `list_test.go` 586 lines; names with "and"; `config_test.go` no ENOTDIR case; no direct `UnreadableReason` table test in `internal/store`.
- refactor-advisor — `Prune`/`autoPrune` each own a delete loop (ctx check, `NotDeleted`, sweep differ) → one `deleteAll` helper; `pruneWarning`/`pruneWarningAbsolute` + `Warnings()`/`WarningsAbsolute()` store one fact twice; `scanFolder` compose method (`isOrphanManifest`, `parseSnapshotName`); `storeSnapshot` returns two adjacent strings; `Pruned` mixes real-run and dry-run fields; `config/parse.go` per-setting boilerplate and a 190-char line; snapshot copy builders spread over four files → one `refusals.go`; `sync.go` RunE ~70 lines; `"quarry: warning: "` prefix literal in three places and `snapshots` re-implements `emitReport`; `reportPruned` six parameters; small render/JSON duplications; `append([]string{}, …)` ×3.

### NIT
- correctness-reviewer — `parse.go:231` dedupe drops only descendants reported after their ancestor (`[foo.bar]` then `[foo]` warns twice).
- arch-reviewer — `snapshots_prune.go:69` `pruned.Keep == 0` as the refusal sentinel; `sync.go:100-108` "error beside a rebuilt store" rule in delivery; store dir layout repeated in help strings.
- test-reviewer — SCENARIO-29/32 acceptance tests run `sync --quicken <dir>` where the spec's When is bare `quarry sync`; mismatch JSON test seeds one snapshot; `snapshots --json` has no cmd-level refusal-prints-no-document test; `futureIDs` assumes the clock is before 2099; small assertion/style items.
- refactor-advisor — `UnreadableReason` doc "call only for faults that carry a reason"; `originRefusals` closures could be two remedy strings.

### Could not check
- A case-sensitive volume (EqualFold fallback judged safe there); a real SIGINT against prune/auto-prune (cancel injected); the Locked history fault with a real cross-process lock; root-run CI (every chmod test skips).

### Strengths
- Dependency rule clean: `snapshot` imports no `report`/`config`/`cli`; `config` imports only platform packages; wiring only in `cmd/quarry/run.go`.
- A failed sync deleting nothing is structural (one `autoPrune` call site at `importVerified`'s success tail); nothing outside the folder or non-regular is removable.
- PRE-01 neutrality verified (zero hunks in the protected test files).
- Fault injection uses real error shapes (`*fs.PathError{EACCES}`, real chmod, driver-shaped duckdb errors); control arms differ in one variable; whole-document JSON pins.

### Verdict: BLOCKED
1 BLOCKER, 4 MAJOR (correctness 5, one shared with test-reviewer). arch PASS WITH FOLLOW-UPS; refactor PASS WITH FOLLOW-UPS; test (cmd) PASS WITH FOLLOW-UPS; test (internal) BLOCKED on the shared MAJOR.
