---
id: SCENARIO-10
status: open
---

# SCENARIO-10: Net worth today

Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_networth_test.go` `Test_run_networth_prints_todays_balances_by_type_and_currency_with_a_total_in_the_reporting_currency`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_networth_test.go` `Test_run_networth_lists_cad_and_usd_separately_with_one_total_each_in_native_mode`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/... ./internal/cli/ -run 'NetWorth|Networth|networth|reads_|row_reads' && go test ./cmd/quarry/ -run 'networth|Networth|read_commands|usage|help|skill|config|home_is_unset|older'`
Mutation checks: native totals never add across currencies in `report.NetWorth` total → `Test_networth_native_totals_each_currency_on_its_own`; zero-balance row omitted from text only in `renderNetWorth` → `Test_render_networth_omits_a_zero_row_that_json_keeps`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`internal/report`; duckstore adapter, cli and docs ride with it); absorbs SCENARIO-13 (FOLD)

Deviations from the orchestrator delta (decide before dispatch):
- **SCENARIO-13 FOLDs here.** `reporting.currency = native` reaches native mode with no flag (`config/parse.go:253`, shared `currencyFlag.resolve` `cli/currency.go:56-68`), so S10 must render snapshot-native or ship an unruled interim. S13's Gherkin is snapshot-only, so S10 delivers it whole. History-native moves to S12 for the same reason.
- **SKILL §4 row moves to SCENARIO-15.** `Test_every_quarry_name_the_skill_uses_exists` (`cmd/quarry/run_skill_drift_test.go:290`) checks `--as-of` and `--since` in that code span against networth's help. They land in S15 and S12. The description, the :73 deletion and PRD L169 stay here.

Flags after S10: `--currency` (reportCurrencyHelp: CAD, USD, native) and global `--json`. No `--as-of` (S15: `AsOfError.Error` at `report/asof.go:27-32` hard-codes "holdings are valued"), no `--since`/`--until` (S12). Long and Example ship verbatim even though they name the later flags. No test checks Example flags.

Contract: `quarry networth` → stdout gets caption `Net worth on <today>, amounts in <CUR>` (native: no ", amounts in X"), a blank line, then the table `Type Currency Balance In <CUR>` (native: no In column). Rows are type×currency with balance ≠ 0.00, sorted by type, then currency. Converted mode has one `Total` row with the In total. Native has one `Total` row per currency, CAD first. stderr gets config warnings only. Exit 0. Refusals reuse existing lines: positional arg / bad `--currency` / bad config currency → 2; no store, older format, HOME unset, interrupted → 1.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_networth_test.go` — both acceptance tests through `runWith(..., []string{"networth"[, "--currency","native"]}, spendEnvAt(..., holdingsClock()))`. Fixture via cmd store helpers: chequing CAD and USD, credit_card CAD (negative), brokerage USD with a priced holding, plus a closed account and a not-in-reports one. Text asserted byte-exact.
- [x] Step 2: signature-only stubs. `store.NetWorthParams`/`store.NetWorth`/`store.NetWorthRow` after `internal/store/store.go:161-175`. `report.Store.NetWorth` at `internal/report/store.go:26-27`; `(*duckstore.Store).NetWorth` in new `duckstore/networth.go`; `NetWorth` on `report/fakes_test.go:11-38` `fakeStore` and on `cli/fakes_test.go:17-35` `fakeReportStore` (it embeds a nil interface, so it would panic). `(*report.Server).NetWorth`; `newNetWorthCommand` registered at `internal/cli/root.go:32`. Red at the text assertion.

### Build
- [x] Step 3: `duckstore/networth.go` `(*Store).NetWorth`, twin of `duckstore/holdings.go:13-67`. One open; `v_net_worth WHERE date IN (<params.Dates>)`; `ORDER BY date, type, currency` (alphabetical: account currency is CAD|USD only, `importer/accounts.go:27`); DECIMAL(38,2) → `*big.Int` cents, NULL converted → nil. Tests in `duckstore/net_worth_read_test.go`, rows: every column; NULL `balance_cad` → nil; types given in both orders; two dates read in one call; a date with no rows → empty; a transaction dated after the asked day not in its balance; amount past int64. Fault coverage: add a `NetWorth` row to `duckstore/read_faults_test.go:20-50` `rowReads` (open, query, scan, close).
- [x] Step 4: new `internal/report/networth.go`, twin of `report/holdings.go:15-154`. `NetWorthRequest{AsOf, Currency}`, `NetWorth{Dates []NetWorthDate{Date, Rows, Totals}, AsOf, Currency}`, `Converted(row)`, `(*Server).NetWorth`. The store is read once, for `[]time.Time{AsOf}`. Snapshot = exactly one `Dates` entry, even with no rows. Converted total = the sum of non-nil converted values, one total in the reporting currency, none when nothing converts. Native = one total per currency, in currency order (`nativeRank` reused). Tests in `report/networth_test.go`: reads the store once with the day; totals arms (sum, negative, zero only, nil left out, past int64, no total); `Test_networth_native_totals_each_currency_on_its_own`; refusal trio twin of `report/holdings_test.go:230-260` (open refusal, `networth interrupted`, other failure unchanged).
- [x] Step 5: new `document/networth.go` `NetWorth`/`NewNetWorth`, twin of `document/holdings.go:29-89`. `as_of`/`since`/`until` are `*string` and always present (snapshot: as_of set, since/until null). `dates`/`balances`/`totals` are never null; `balances` keeps zero rows. `converted_balance` is null in native mode and when there is no rate. `totals` follows the text order. Tests in `document/networth_test.go`: a `map[string]any` read-back pins null vs absent keys and `[]` on a zero-row date; native vs converted `converted_balance`.
- [x] Step 6: new `cli/render_networth.go` `renderNetWorth`, twin of `cli/render_holdings.go:30-77`. Converted and native captions; header with and without `In <CUR>`; zero row omitted; Total row(s) as in the contract; no Total row when there are no totals; nil converted cell left blank (interim, 14b). Tests in `cli/render_networth_internal_test.go`: one row per arm, plus `Test_render_networth_omits_a_zero_row_that_json_keeps`.
- [x] Step 7: new `cli/networth.go` `newNetWorthCommand`, twin of `cli/holdings.go:16-80` minus `--as-of`/`--account`. Use/Short/Long/Example verbatim from Surface & Copy; `currency.bind(cmd, reportCurrencyHelp)`; `Args: currency.args`; day = `report.Today(now())`; `emitReport` with `withConfigWarnings`. New `cli/json_networth.go`, twin of `json_holdings.go`. Tests:
  - cmd `--help` pin (Long, Example, `--currency` line) in `cmd/quarry/run_networth_surfaces_test.go`.
  - `--json` cmd test: zero row kept, closed account counted, not-in-reports absent, credit card negative.
  - n/a, with owner: future-dated (Step 3), excluded transaction and investment with no holdings (pinned by the view, S06/S09), no rate (14b), empty (16).
  - All-commands rows: `run_read_usage_test.go:44,80`; `run_read_refusals_test.go:52,102,228`, plus `assertRefusesAnOlderStore(t, "networth")` beside `:168`; `run_spend_refusals_test.go:90`; `run_config_test.go:222`; `run_usage_test.go:257`; root help `run_status_test.go:146-160` (`  networth    Show net worth today or at each month end, by account type and currency`, between mcp and recurring); `cli/currency_test.go:23`; `cli/report_help_test.go:186-193` (`reportCurrencyHelp`).
  - Docs, verbatim from *Changes to existing surfaces*:
    - `plugin/skills/quarry/SKILL.md:3` description: insert `net worth today or over time; ` after `for account balances; `; the exclusion becomes `for gains or ACB beyond`. Re-pin `run_skill_text_test.go:163-166` `skillFrontmatter`.
    - Delete `SKILL.md:73`; re-pin `skillSection7` at `:219`.
    - `docs/initial-prd.md:169` networth row.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols; add networth to the command lists in `internal/cli/root.go:8` and `internal/report/doc.go`. Trim on touch: STATE open-debt comment MINORs.

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4c-networth` → tick SCENARIO-10, and SCENARIO-13 "delivered by SCENARIO-10" with its test; STATE.md rewrite.

## Handoff

**Binding decisions:**
- One port method, `report.Store.NetWorth(ctx, store.NetWorthParams{Dates})`: one open, `v_net_worth WHERE date IN (...)`, ordered by date, type, currency. S12 passes month ends. S14a, S14b and S16 add their facts (unvalued holdings, first rate, first transaction) as fields of `store.NetWorth` read in that same open — one read per command.
- The day comes from the CLI's injected clock (`report.Today(now())`), never `current_date` in SQL. Tests depend on it.
- Snapshot = exactly one `NetWorth.Dates` / JSON `dates` entry, even with no balances. S16 and S17 inherit this.
- JSON `as_of`, `since` and `until` keys are always present. S12 fills since/until and nulls as_of.
- Native is reachable through config, so every mode renders it. S10 does snapshot (S13 folded). S12 must render history-native (`Month end  Currency  <types…>  Total`) in its own plan.
- The converted total sums only non-nil converted values, in the reporting currency. 14b adds the `Total <CUR>` total and its row.
- SKILL §4 row → S15, the scenario where the last of `--as-of`/`--since` lands.

**Left unbuilt:**
- `--as-of` and the networth `AsOfError` noun → S15. `--since`/`--until` → S12.
- The `no rate` cell, the `Total <CUR>` row and warning 6 → 14b. Until then a nil converted cell is blank and silently left out of the total.
- Empty-result warnings → S16. Until then the output is caption plus header, no Total.
- Warnings 3-5 → 14a. MCP `net_worth`, SKILL §9 → S17. SKILL §4 row → S15.

**Traps:**
- `v_balances_daily` runs only through DuckDB's real today, so cmd fixtures need a past clock (`holdingsClock()`, 2026-03-12).
- The drift test scans SKILL code spans, so adding the §4 row before both flags exist reddens `Test_every_quarry_name_the_skill_uses_exists`.
- The help Example names `--as-of`/`--since` before they exist; no test checks it. Do not trim the ruled copy.

## Phase report

Run B1 done (Steps 3-7), both acceptance tests green, Narrow loop green (duckstore, report, document, cli, cmd). Sweep/Verify (Steps 8-9) left for V; lint not run yet.

Files:
- `internal/store/duckstore/networth.go`: `(*Store).NetWorth` (one open, `date IN (CAST($n AS DATE)...)`, HUGEINT cents). Tests `net_worth_read_test.go`; `NetWorth` row added to `rowReads` in `read_faults_test.go`. Empty `Dates` is not guarded (`IN ()`); the report always passes one date.
- `internal/report/networth.go`: `NetWorth{Dates []NetWorthDate{Date, Rows, Totals []NetWorthTotal}, AsOf, Currency}`, `Converted`, `(*Server).NetWorth` (one read, snapshot = one Dates entry), `nativeNetWorthTotals` reuses `nativeRank`. `report/fakes_test.go` fakeStore gained `netWorth`/`gotNetWorth`/`netWorthReads`. Tests `report/networth_test.go`.
- `internal/report/document/networth.go` (`NetWorth`, `NewNetWorth`; the plan said `document/` = this dir) + `networth_test.go`.
- `internal/cli/render_networth.go`, `json_networth.go`, `networth.go` (Short/Long/Example verbatim; warnings nil, Steps 14a/16 add them), tests `render_networth_internal_test.go`.
- cmd: `run_networth_surfaces_test.go` (help pin), `run_networth_json_test.go` (converted + native), all-commands rows added in `run_read_usage_test.go`, `run_read_refusals_test.go` (no store, config EUR, older store, interrupt), `run_spend_refusals_test.go` (HOME), `run_config_test.go` `readCommandArgs`, `run_usage_test.go` (needs value, hint), `run_status_test.go` root help, `internal/cli/currency_test.go`, `report_help_test.go`.
- `seedNetWorthStore` gained a zero-net Savings account (omitted from text, kept in JSON) and manual investment txn/split SourceID 99 (spendRows numbers its own 1..n).
- Docs: SKILL.md description (net worth added, exclusion `gains or ACB`), `:73` deleted, `skillFrontmatter`/`skillSection7` re-pinned; PRD L169.

Mutations (both red, restored, diff clean): `sums[row.Currency]` -> `sums["CAD"]` in `nativeNetWorthTotals` -> `Test_networth_native_totals_each_currency_on_its_own` (expected CAD 125/USD 250, got CAD 375); `Sign() == 0` -> `== 7` in `renderNetWorth` -> `Test_render_networth_omits_a_zero_row_that_json_keeps` ("should not contain savings").

For V: lint, doc comments, `internal/cli/root.go:8` and `internal/report/doc.go` command lists, STATE open-debt trims, full verify, spec tick, STATE rewrite.
