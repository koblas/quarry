---
id: PRE-01
status: open
---

# PRE-01: Report-pipeline de-duplication and the R3 reason phrase moved to `internal/store`

Cadence: code-first — no mandatory test-first item: no bug fix, no write-safety guard, no atomic adapter
Acceptance test: none — behaviour-neutral pre-step; proof is P2c-12 (the existing black-box suite passes with no test-file hunk)
Narrow loop: `go test ./internal/store/... ./internal/report/ ./internal/cli/` per batch; end of each B run also `go test ./cmd/quarry/ -run 'spend|cashflow|refuse'`
Mutation checks: none — no new guard or branch; every moved branch is already pinned by an unchanged test, and a new test would break P2c-12's ±0
Runs: B1 (1-2) | B2 (3-4) | V (5-6)
Size: OWNS A RUN — 4 batches, 1 feature package (report) + store/duckstore + store + cli; no cmd change

Neutrality rule for every step: no `*_test.go`, `testdata` or `cmd/` hunk. Grep confirms no test names a symbol this plan renames or moves (the white-box `render_*_internal_test.go` call only `renderSpending`/`renderCashFlow`, which stay). Needing to touch a test = behaviour change → stop and report (P2c-12).

## Implementation Plan

### Build
- [x] Step 1: new `internal/store/duckstore/filter.go` ← `spending.go:14-58` (`accountFilter`, `and`, `args`, `readArgs`, `marks`, `transactionRangeQuery`) + `spending.go:189-192` `civilDay`; extract `transactionRange` from the duplicate blocks `spending.go:176-182` and `cashflow.go:77-83` (read stays the LAST statement in both); reachability note on `ErrUnsupportedGrouping` `spending.go:136-137` and `ErrUnsupportedPeriod` `cashflow.go:18-19` (cli's `--by` tables refuse every other value before a read). Unchanged: `duckstore/spending*_test.go`, `cashflow_test.go`, `read_faults_test.go`
- [x] Step 2: report + store —
  (a) one fill helper in `report/period.go` replacing `spending.go:88-109` `fillMonths` and `cashflow.go:66-89` `fillPeriods` (series × Totals currencies, store row else zero row, Partial from the period);
  (b) `DefaultWindow` `spending.go:50-58` → `window.go`;
  (c) drop `period.First` and `period.Last` fields `period.go:12-18,43-48` — no reader (grep over report+cli; positive control `span.First` hit); the local `last` stays for Partial;
  (d) new method `(*store.OpenError).UnreadableReason(at string) string` (`store/open.go:16-35`) returning the bare reason phrase for NotDuckDB/Permission/Locked/Other, called by `report/refusal.go:54-61` `storeRefusal`. Missing/OtherFormat arms and every prefix/remedy stay in `report`. `exhaustive` is on (`.golangci.yaml:3` `default: all`, no `exhaustive:` settings) → name every `OpenFault`; no arm the existing refusal cases do not execute (group Missing/OtherFormat into Other's arm, or a lookup table with Reason fallback). `internal/store` stays ±0 tests: `report/refusal_test.go:19-80` covers it via `-coverpkg`.
  Unchanged: `report/refusal_test.go`, `spending_month_test.go`, `cashflow_test.go`, `window_test.go`, `spending_test.go`, `status_test.go`
- [x] Step 3: cli table — new `internal/cli/render_table.go` `renderTable` (caption, padded columns — first two left, rest right via `render_accounts.go:75-84` `padLeft`/`padRight` — trailing Status cell unpadded, written only when non-empty) called by `render_spend.go:19-64` and `render_cashflow.go:21-57`; rename `spendingTotalLabel`/`spendingPartialStatus`/`spendingAccountsCaption` (`render_spend.go:11-15,66-76`) to report-neutral names; `time.DateOnly` for `render_spend.go:20`, `render_cashflow.go:22`, `report/window.go:31` `layoutDay`, `json.go:13` `jsonDateLayout` (keep the name, value `time.DateOnly`). Out of scope: `render.go:262,290,323` (sync validation output, not the report pipeline). Unchanged: `render_spend_internal_test.go`, `render_cashflow_internal_test.go`, `spend_test.go`, `cashflow_test.go`
- [x] Step 4: cli command tail —
  (a) `window.go:11-38` `windowFlags` → one report flag struct also owning `--account` (`spend.go:15,79`, `cashflow.go:38,96`), registered in today's order with today's help strings;
  (b) one shared tail for `spend.go:66-74` / `cashflow.go:83-91` (warnings → `renderResult` → `emit`) with ONE `// unreachable:` comment covering both callers' reasons;
  (c) `spendAccountDocument` `json_spend.go:19-23` → `accountDocument` + one builder for `json_spend.go:75-78` / `json_cashflow.go:55-58`;
  (d) `allLeftOut` predicate from `empty_window.go:42-54`;
  (e) unexported command-name consts in cli (`spend.go:18,86`, `cashflow.go:41,103`) and in report (`spending.go:63,69`, `cashflow.go:48,54`).
  Unchanged: `report_help_test.go`, `spend_window_test.go`, `spend_account_test.go`, `spend_empty_test.go`, `json_*_internal_test.go`, cmd `run_*spend*`/`run_cashflow*` tests

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new/moved symbol (`go doc ./internal/store OpenError` reads as the contract); `filter.go` has no doc of history

### Verify
- [x] Step 6: full verification (`.claude/rules/agent-briefs.md`) incl. `go test -race ./internal/store/... ./internal/report/ ./internal/cli/`; `test-stats.py --base <start> --changed` → ±0 everywhere (expect no rows); `git diff --exit-code <start> -- '*_test.go' cmd/ ':(glob)**/testdata/**'` empty; no `--json` field, help string or golden changed; `spec-check.py phase2c-snapshots-config` plain (no tick — PRE-01 is traced in `METRICS.md` only); create `docs/specifications/phase2c-snapshots-config/STATE.md` (first 2c unit); do NOT edit 2b `STATE.md` (orchestrator closes that debt at SHIP, surface #9)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The R3 reason phrase is `(*store.OpenError).UnreadableReason(at string) string`: takes the store's display path (caller abbreviates with `homepath.Abbreviate`), returns the bare phrase — `the file is not a DuckDB database`, `permission denied`, `another program has it open for writing`, or Reason with every Path replaced by `at` — with no `cannot read the store at …:` prefix and no remedy. SCENARIO-15 (CF2), 16 (`snapshots` warning), 21 (prune refusal) render it from `snapshot`, which may not import `report`.
- `internal/store/duckstore/filter.go` owns `accountFilter`, `readArgs`, `civilDay`, `transactionRangeQuery`, `transactionRange` — a new windowed read reuses them, never a copy.
- One fill helper in `report/period.go` for every period-series report; one report flag struct in `internal/cli/window.go`; one spend/cashflow tail; `accountDocument` is the `account_filter` entry for every report.

**Left unbuilt** — named so nobody assumes it exists:
- `store.CashFlowFigures` — NOT neutral: embedding it in `store.CashFlowRow`/`CashFlowTotal` rewrites composite literals in `internal/cli/cashflow_test.go` (4) and `internal/store/duckstore/cashflow_test.go` (10), both on P2c-12's no-hunk list. V records it in 2c STATE `## Open debts`, unowned.
- `render.go:262,290,323` date literals — outside the report pipeline; stay.

**Traps** — things that look right and are not:
- `transactionRange` must stay the last statement of `Spending` and `CashFlow`: the tag-fault tests count statements positionally (`passQueries`).
- The shared tail's `// unreachable:` reason must hold for both callers — spend's (marshalDocument only) and cashflow's (`savings_rate_pct` finite) — or the claim is false for one.
- cli `Use:` word and report's refusal word (`spend interrupted`) live in two unexported consts; they must stay equal.
- `exhaustive` rejects a partial `OpenFault` switch even with `default:`; an arm only Missing/OtherFormat reach is uncovered (report handles those first) and fails `uncovered-diff.py`.
- Cobra renders flag help from registration; keep `--by` first, then `--since`, `--until`, `--account` exactly as today.

## Phase report

Run V (steps 5-6) — done. `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite rc=0; `uncovered-diff.py` 0 uncovered, 1 declared unreachable (`internal/cli/output.go` `emitReport`); `go test -race ./internal/store/... ./internal/report/ ./internal/cli/` ok; `test-stats.py --base b9bb83c --changed` TOTAL 0 (+0); neutrality diff empty; `spec-check.py phase2c-snapshots-config` OK. `STATE.md` created (`store.CashFlowFigures` unowned Open debt). 2b STATE.md untouched. Nothing left.
