---
id: SCENARIO-08
status: open
---

# SCENARIO-08: Spend converts to the reporting currency by default

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_spend_fx_test.go` `Test_run_spend_converts_every_split_to_cad_by_default`
Acceptance test (SCENARIO-14, folded): `cmd/quarry/run_spend_fx_test.go` `Test_run_spend_takes_its_currency_from_the_config_unless_the_flag_names_one`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)spend|currency|cashflow_total'`
Mutation checks: NULL arm of the converted source (a split with no converted cell keeps its own currency) → `Test_spending_keeps_a_split_with_no_rate_on_a_row_of_its_own_currency`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (report) + duckstore Spending + cli

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_spend_fx_test.go` (new) — both acceptance tests through `runWith`, store from `replaceStoreWithRates` (`run_sql_fx_test.go:32`), config via `writeConfig` (`run_config_test.go:56`). 08: CAD + USD spending with a rate before the earliest split; a pair of USD splits that each convert to a half cent (Total reachable only by rounding each split before summing), and at least one USD row whose CAD figure differs from its native one; text caption ends `, amounts in CAD`, every row and the single Total are CAD; `--json` `currency` is `"CAD"`. 14: the six Examples rows (unset→CAD, `USD`→USD, `native`→native, `usd`→USD, config `USD` + `--currency CAD`→CAD, malformed file + `--currency CAD`→CAD with empty stderr), each crossed with text and `--json`.
- [ ] Step 2: no stubs needed (tests call only `runWith` and existing helpers); confirm both fail at the caption/`currency` assertion, not at setup.

### Build
- [ ] Step 3: `internal/store/store.go:493-500` `SpendingParams.Currency money.Currency`; `internal/store/duckstore/spending.go:12-86,97-137` — one source relation chosen by currency feeds every query: category/payee (`:18-34`), month subquery (`:37-39`), both halves of the tag UNION (`:47-62`). Native is literally `v_spending`, so its SQL text is unchanged. CAD/USD project `spent_cad`/`spent_usd` as `spent` and the reporting currency as `currency`, except where the converted cell is NULL: that split keeps its native `spent` and `currency` in the same statement, so a Total never mixes. Rows and totals stay one `QueryRows`. There is no new fallible call, so the existing fault tests at `spending_test.go:197-228` cover it. New `spending_fx_test.go` (via `newStoreWithRates`, `views_fx_test.go:42`) pins one arm per case:
  - CAD, USD, and native equal to the pre-change rows on a rated store.
  - The NULL arm in CAD and in USD modes: a split before the first rate, and a non-CAD/USD currency stays native.
  - Per-split rounding, with a sum-then-round control.
  - Weekend (Friday rate) and future-dated (latest prior rate).
  - Closed account converts.
  - `--by tag`: rows and the totals half each converted.
  - `--by month`.
  - Payee rank flipped only after conversion.
  - A category netting to zero only after conversion drops while its Total keeps it.
  - `AccountIDs`: one USD account in CAD gives a CAD Total.
  - Totals CAD first.
- [ ] Step 4: `internal/report/spending.go:13-17` `SpendRequest.Currency`, `:29-43` `Spending.Currency`, `:54-79` `Spend` passes it into `SpendingParams` and echoes it on the result (`spending_test.go` row: requested currency reaches the store params and the result). In `internal/cli/spend.go:59,69`, thread the resolved currency instead of `_`. `internal/cli/render_table.go:51-55` `windowCaption` gains a currency: `, amounts in CAD`/`USD` is appended, and native leaves it unchanged. Callers `render_spend.go:35` pass `s.Currency`; `render_cashflow.go:32`, `render_recurring.go:80` and `render_anomalies.go:47` pass `money.Native`. `json_spend.go:9-17,84-92` `currency` right after `by` (`Currency.String()`). Tests:
  - `windowCaption` classes CAD/USD/native in its own internal test.
  - `json_spend_internal_test.go`: top-level key ORDER pinned (`since,until,by,currency,account_filter,rows,totals,warnings`), identical in CAD and native.
  - `fakes_test.go:40`: `gotSpending` assert `Currency` per resolver arm (flag, config, flag-beats-config).
- [ ] Step 5: `internal/cli/spend.go:23-25` Long: the opening is its own paragraph, `Show how much you spent, grouped by category, payee, tag or month.`, then a blank line, then P verbatim (spec `## Surface & Copy` → *Changes to existing surfaces*), held as one shared const in `internal/cli/currency.go` for 10 to reuse. Pin it verbatim in `report_help_test.go:14-42`. Cmd edge matrix in `run_spend_fx_test.go`, a table over {closed USD account, one USD `--account`, empty window, future-dated with `--until` past it, weekend date} × {CAD, USD, native} × {text Total/caption, `--json` `currency` + totals}:
  - native × weekend and native × future-dated are n/a: the native source never reads `usd_cad`.
  - empty window: `currency` is still set, no totals, the existing empty note only.
  - Plus `Test_run_spend_json_reads_back_with_every_amount_in_the_reporting_currency`: `encoding/json` decode; each row/total `currency` == `.currency`; rows sum to the Total (`--by category`).
  - Reached and unchanged, so not re-pinned: cross-currency transfer excluded (view), and empty fx_rates all-CAD (identity arm, covered in Step 3).
  - Before-first-rate and no-rates warnings are owned by 12/13.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments on `SpendingParams.Currency`, `SpendRequest.Currency`, `Spending.Currency`, `windowCaption`, and the source helper. Bump the caption pins that now gain `, amounts in CAD`:
  - `cmd/quarry/run_spend_empty_test.go:16`
  - `run_spend_account_test.go:38,62,82`
  - `run_spend_by_test.go:38,69,101`
  - `run_spend_window_test.go:26,32,38,44`
  - `run_spend_test.go`
  - `internal/cli/spend_test.go:48,64`
  - `spend_account_test.go:62`
  - `spend_tag_test.go:33`

  Bump the JSON pins that gain `currency`: `run_spend_json_test.go:38`, `spend_test.go:116`, and `json_spend_internal_test.go:37,80,123,158` (zero `Currency` renders `"native"`).

### Verify
- [ ] Step 7: run the full verification and `spec-check.py phase2f-fx`. Tick SCENARIO-08 with its acceptance test. Tick SCENARIO-14 as `delivered by SCENARIO-08` with `Test_run_spend_takes_its_currency_from_the_config_unless_the_flag_names_one` last on the line. Rewrite STATE.md.

## Handoff

**Binding decisions:**
- **A split with no converted cell stays on a row of its native currency, chosen in the same SQL statement.** A mixed Total is impossible from 08 on. 12/13 add only the count, first rate and warnings, and must not regroup.
- **Native mode reads `v_spending` exactly as before.** Its byte-identity is the 11 outline's premise.
- **`money.Currency`'s zero value is `Native`.** So `SpendingParams{}` and `SpendRequest{}` default to native, and pre-08 duckstore and report tests stay native unchanged. cli always passes the resolved value.
- **`windowCaption(title, window, accounts, currency)` is the one caption chokepoint.** The suffix is `, amounts in <CAD|USD>`; native adds nothing.
- **Spend JSON `currency` sits right after `by`.** Its value is `Currency.String()`. The key set is the same in every mode.
- **The spend Long opening is its own one-sentence paragraph, then P from one shared const.** 10's cashflow Long reuses that const unchanged.

**Left unbuilt:**
- `CashFlowParams.Currency`, `CashFlowRequest.Currency`, cashflow conversion and the cashflow Long: 10.
- The before-first-rate and no-rates warnings and their counts: 12/13.
- Recurring, anomalies and accounts conversion: 17-19.

**Traps:**
- `render_cashflow.go:32`, `render_recurring.go:80` and `render_anomalies.go:47` pass a literal `money.Native` to `windowCaption`. 10/17/18 must replace it with their report's currency.
- `run_cashflow_invariant_test.go:76` stays green with no `--currency native` pin, because `replaceStore` seeds no `fx_rates`. Its USD splits keep NULL `spent_cad`, stay USD, and match native cashflow. It breaks before 10 lands if rates are seeded into that fixture, or if the NULL arm drops or relabels rows.
- On a store built with `replaceStore` (no rates), CAD and native give the same numbers. A "converted" or "native is identical" pin proves nothing there; use `replaceStoreWithRates`/`newStoreWithRates`.
- Until 12/13, a CAD-mode report on an unrated store shows USD rows under `amounts in CAD` with no warning. That is intended interim behaviour; do not "fix" it by dropping rows.
- Internal render tests that build a `report.Spending` with zero `Currency` render native: no caption suffix and `"currency":"native"`.
- `-run` is case-sensitive; keep the narrow loop's `(?i)`.

## Phase report
