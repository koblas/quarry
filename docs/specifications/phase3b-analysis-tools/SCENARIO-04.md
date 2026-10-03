---
id: SCENARIO-04
status: open
---

# SCENARIO-04: An unknown or ambiguous account is refused without its name reaching stderr (folds SCENARIO-05)

Cadence: code-first (no mandatory test-first item; phase3a REVIEW-02's two findings carry no constructible `Failure:`, so they are not bug fixes)
Acceptance test: `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_refuses_an_account_without_its_name_on_stderr`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_mcp_store_faults_test.go` `Test_run_mcp_logs_only_the_withheld_line_for_a_store_read_fault`
Narrow loop: `go test ./internal/report/ ./internal/mcp/ -run '(?i)refusal|logLine|account|fault'` then `go test ./cmd/quarry/ -run 'Test_run_mcp'`
Mutation checks: Kind discriminator in `logLine` (zero `Fault` is `OpenFaultOther`) → `Test_logLine_keeps_a_refusal_of_no_kind_verbatim`; `OpenFaultOther`-only arm → the five verbatim fault rows of `Test_logLine_classifies_each_refusal`; `Arg`/`IDs` never in the line → `Test_logLine_never_carries_the_callers_account_text`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/report`); `internal/mcp` is a delivery surface (imports report, like `internal/cli`; S03 had the same shape), `cmd/quarry` is tests only. Absorbs S05 (table over 5 tools; S09/S10 add cash_flow, recurring_charges, anomalies rows).

Surface (spec §4.2, §6.1 rows 10-12; verbatim, no new copy):
- unknown account client text `no account named "X"; call describe_schema to list the accounts`; ambiguous client text unchanged (`2 accounts are named "Visa"; pass one of their ids instead: a1, a2`)
- stderr (`quarry: mcp: <tool>: …`): `refused the call's accounts: one names no account; details went to the client only` / `refused the call's accounts: one names more than one account; details went to the client only` / `cannot read the store at <~ path>; details went to the client only` for `OpenFaultOther` only (open- or statement-time); isError for all, exit unchanged
- missing / other-format / not-DuckDB / permission / locked stay verbatim on stderr; CLI stdout/stderr byte-identical (pins below)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_spending_test.go` (after `:121` window test; consts `:14-25` hold `spendingLogPrefix`) `Test_run_mcp_spending_refuses_an_account_without_its_name_on_stderr` — rows: unknown `Nope`, empty `""`, ambiguous (two accounts named `Visa`, fixture via `accountsBuilder("Visa","Visa")` `run_mcp_status_test.go:139`; ids read from the store). Each row carries its own `absent []string` (name, ids; for the `""` row the quoted `""` form: `NotContains(stderr, "")` always fails). Asserts isError, exact client text, exact stderr (`spendingLogPrefix`+class line), and every `absent` string missing from stderr. Red at the stderr assertion (today the CLI wording is logged verbatim)
- [x] Step 2: `cmd/quarry/run_mcp_store_faults_test.go` (new) `Test_run_mcp_logs_only_the_withheld_line_for_a_store_read_fault` — table over query, describe_schema, sync_status, data_quality, spending, every row through the real binary wiring. Open-time rows: a directory made at `storePathUnder(home)` (Stat passes, so not Missing; `refusal_test.go:67` models the "Is a directory" reason) — developer verifies it classifies `OpenFaultOther` and not NotDuckDB/Permission; if it does, all five tools get a real row here, `query` included. Statement-time rows via `editStore` (`run_read_refusals_test.go:241`), each confirmed `OpenFaultOther` first (all four sites wrap with `openFault`: `status.go:71,92`, `schema_read.go:57-89`, `findings_read.go:88`, `spending.go:152`): sync_status `DELETE FROM import_runs` (`run_mcp_status_test.go:118`); spending drop of the relation `spending.go:126`'s query reads (`DROP VIEW v_spending`); describe_schema `DROP TABLE categories CASCADE` (`schema_read.go:27`); data_quality `DROP TABLE finding_items` then `findings` (`run_status_findings_test.go:235-236`). `query` has no statement-time row (a bad statement is `QueryError`); if the directory fault does not classify Other, its row falls back to an `internal/mcp` fake-store open-time `OpenError` (Step 5) and the phase report says so. Assert client text = the CLI twin's `cannot read the store at ~…: <reason>; run quarry sync to rebuild it` and stderr = the withheld line, nothing else. Red at stderr. No signature stubs: tests compile against existing API

### Build
- [x] Step 3: batch 1, `internal/report/refusal.go:14-24,62-71` — `RefusalError` gains exported parts so a surface can word/classify without CLI text: `Kind` (zero value = generic, then unknown-account, ambiguous-account, store), `Arg`, `IDs` (sorted copy), `Fault store.OpenFault`, `At` (the `~` form `storeRefusal` computes at `:44`). Set in `unknownAccountRefusal`, `ambiguousAccountRefusal`, `storeRefusal` (`:59`; the `readRefusal` interrupted refusal `:31` stays generic). `Error()` unchanged. Tests in `internal/report/refusal_test.go` (pins parts for every `OpenFault` incl. a path outside home; unknown with `""`; ambiguous ids sorted, caller's slice not aliased) and `spending_account_test.go:88-126` / `cashflow_test.go:217-229` (parts via `errors.AsType`). Existing text pins must stay green untouched: `report/*_account_test.go`, `internal/cli/spend_account_test.go:165,175`, `cmd/quarry/run_spend_account_test.go:118-128`, `run_recurring_refusals_test.go:38-51`, `run_anomalies_refusals_test.go:32-45`, `run_cashflow_refusals_test.go:44`, `run_read_refusals_test.go`
- [x] Step 4: batch 2, `internal/mcp/result.go:24-28,51-60` `logLine` + consts, `internal/mcp/accounts.go` (new) `accountRefusal(err)`, `internal/mcp/spending.go:32-34` call site — `logLine` classifies by `RefusalError.Kind`: unknown/ambiguous → two class consts beside `windowRefusedLog`; `OpenFaultOther` store refusal → withheld line from `At`; every other fault and generic kind verbatim (`loggedError` still first). `accountRefusal` words the unknown-account client text, leaves ambiguous and every other error unchanged; its error must still unwrap to the `RefusalError` so `logLine` classifies it, and S09/S10 call it unchanged at their `Spend`/`CashFlow`/`Recurring`/`Anomalies` error sites. Unit tests (white-box, `internal/mcp/result_internal_test.go` or new `log_classes_internal_test.go`): `Test_logLine_classifies_each_refusal` (rows: unknown, ambiguous, OpenFaultOther open-time and statement-time shapes, and each of Missing/OtherFormat/NotDuckDB/Permission/Locked verbatim), `Test_logLine_never_carries_the_callers_account_text` (distinctive name and ids absent), `Test_logLine_keeps_a_refusal_of_no_kind_verbatim`, `Test_accountRefusal_words_unknown_and_leaves_the_rest` (`""`, quote/`%`-bearing arg, ambiguous, store refusal, non-refusal error). Fold S03 checkpoint debts here: `internal/mcp/window.go:15,41` and `internal/report/window.go:54` `// unreachable:` reasons cite how established (`parseWindow`/`parseDateBound` are the only `WindowError` constructors); add a distinctive-`Value` case to `window_internal_test.go:72` asserting absence from `logLine`
- [x] Step 5: batch 3 — flip pins, finish the S05 table. `internal/mcp/query_refusal_test.go:99-101` row "a store unreadable for another reason": client `want` unchanged, add per-row `wantLog` (loop at `:109-121` asserts `logPrefixQuery+c.want` for all rows today) = withheld line for that row, verbatim for the rest. `cmd/quarry/run_mcp_status_test.go:118-136`: stderr assertion becomes the withheld line (client text and CLI parity unchanged). `internal/mcp/timeout_test.go:50-68` stalling store returns `OpenFaultOther`; confirm the deadline line still wins (no edit expected). Add the `query` open-time `OpenFaultOther` row (only if Step 2's directory fault does not classify Other: fake store at `internal/mcp`, `query_refusal_test.go` harness). Make Step 2 green. S09/S10 add `cash_flow`, `recurring_charges`, `anomalies` rows to the Step 2 table; record in STATE.md

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported `RefusalError` parts/`RefusalKind` (go doc budget) and on `accountRefusal`; `RefusalError` now holds a slice, so lint/`==` hits on it are in scope

### Verify
- [ ] Step 7: full verification block (`.claude/rules/agent-briefs.md`), `spec-check.py phase3b-analysis-tools`; tick SCENARIO-04 with its acceptance test, and SCENARIO-05 with the "delivered by SCENARIO-04, covers 5 tools" line (test ref last); rewrite STATE.md (drop the two OWNED BY S04 debts and the S03 MINOR; record the flipped pins)

## Handoff

**Binding decisions:**
- `report.RefusalError` stays one type and gains exported parts (`Kind`, `Arg`, `IDs`, `Fault`, `At`); no distinct account type — ~15 tests do `errors.AsType[report.RefusalError]`. Kind zero = generic, because zero `Fault` is `OpenFaultOther`: classifying on `Fault` alone would withhold every non-store refusal
- Classification lives in `mcp.logLine` (defence: a tool that forgets `accountRefusal` still leaks nothing on stderr); `accountRefusal` only rewords the client text. Only `OpenFaultOther` is withheld; the withheld line carries the `~` path (path only, no reason)
- Surface: S09/S10 call `accountRefusal` unchanged; each adds its row to the Step 2 table

**Left unbuilt:** `cash_flow` / `recurring_charges` / `anomalies` rows of the store-fault table and account rows (S09/S10); window wording for them is S03's `windowRefusal`; per-tool config stderr line (S09)

**Traps:**
- `logLine` must keep `RefusalError` text verbatim for the non-Other faults — their `UnreadableReason` is a fixed phrase (`internal/store/open.go:40-44`); withholding them would break `run_mcp_no_store_test.go` and `query_refusal_test.go`
- `query` cannot raise a statement-time `OpenFaultOther`; a `DROP` in the store gives a `QueryError` (classified `rejected`), not a withheld store line
- `go test -run 'Window'` is case-sensitive; narrow loops use `(?i)`

## Phase report

Run B1 (steps 3-5) done; both acceptance tests green, CLI refusal pins untouched and green, `golangci-lint run ./...` = 0 issues (whole repo). Step 6 sweep partly done (lint); V still owns full verification, spec tick, STATE.md, `status: done`.

Production:
- `internal/report/refusal.go:14-50`: `RefusalKind` (`RefusalGeneric` zero, `RefusalUnknownAccount`, `RefusalAmbiguousAccount`, `RefusalStore`); `RefusalError` gains `Kind`, `Arg`, `IDs` (the sorted copy), `Fault`, `At`; set in `storeRefusal`, `unknownAccountRefusal`, `ambiguousAccountRefusal`. `readRefusal`'s interrupted refusal stays generic. `Error()` unchanged
- `internal/mcp/result.go`: consts `unknownAccountLog`, `ambiguousAccountLog`, `withheldStoreLog(at)`; `logLine` -> new `refusalLine` (exhaustive switch; only `RefusalStore` + `OpenFaultOther` withheld)
- `internal/mcp/accounts.go` (new): `accountRefusal(err)` words unknown only (`accountRefusedError` unwraps to the `RefusalError`); `internal/mcp/spending.go:33` calls it
- `// unreachable:` reasons now cite constructors: `internal/mcp/window.go:15,41`, `internal/report/window.go:54`

Tests: `internal/report/refusal_test.go` (store parts per fault + outside home; interrupted = generic), `spending_account_test.go`, `cashflow_test.go` (account parts); `internal/mcp/log_classes_internal_test.go` (new: `Test_logLine_classifies_each_refusal`, `..._never_carries_the_callers_account_text`, `..._never_carries_a_store_reason_in_the_engines_words`, `..._keeps_a_refusal_of_no_kind_verbatim`, `Test_accountRefusal_words_unknown_and_leaves_the_rest`); `window_internal_test.go` distinctive-Value case. Flipped pins: `internal/mcp/query_refusal_test.go` (per-row `wantLog`, test renamed `..._and_logs_its_text_unless_the_engine_wrote_it`), `cmd/quarry/run_mcp_status_test.go` sync_status stderr = withheld line. `timeout_test.go` needed no edit (green).

Mutations (all red, restored): Kind discriminator dropped -> `Test_logLine_classifies_each_refusal/an_account_that_names_none|several` + `Test_logLine_keeps_a_refusal_of_no_kind_verbatim`; withhold every fault -> verbatim rows of `Test_logLine_classifies_each_refusal` (missing, other format, not DuckDB, permission, locked); `Arg` appended to unknown line -> `..._classifies_each_refusal/an_account_that_names_none` + `..._never_carries_the_callers_account_text`; `IDs` appended to ambiguous line -> same two tests, ambiguous rows.

Notes for V: S09/S10 add `cash_flow`/`recurring_charges`/`anomalies` rows to the store-fault table in `cmd/quarry/run_mcp_store_faults_test.go` and call `accountRefusal` at their report error sites. `RefusalError` now holds a slice (not `==`-comparable): grep found no `map[error]` and no `==`/`!=` on it in `internal/` and `cmd/` (build and lint cannot see interface-held comparisons; `errors.Is` with such a target is safe). The plan's "caller's slice not aliased" pin was not added: `slices.Sorted` always allocates and no caller slice reaches the refusal. No uncovered-diff run yet.
