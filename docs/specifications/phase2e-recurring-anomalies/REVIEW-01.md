# Review round 1 — phase2e-recurring-anomalies

Range `0b42411..HEAD`. Full suite rc=0, 0 uncovered. Mutation sample 20/84, 20 killed, 589s.

Triggered: arch (PASS), correctness A report/store (PASS WITH FOLLOW-UPS), correctness B cli/cmd (PASS WITH FOLLOW-UPS), test A report/store (PASS WITH FOLLOW-UPS), test B cli/cmd (BLOCKED), refactor (PASS WITH FOLLOW-UPS). Skipped: api-reviewer, pipeline-reviewer.

## MAJOR
1. test-B — `cmd/quarry/run_spend_refusals_test.go:67` `Test_run_report_commands_refuse_when_home_is_unset`: no H1 rows for `recurring`/`anomalies`. Add both rows (`quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry <cmd> again`).
2. test-B — `internal/cli/render_table.go:31`: no test pins column width for a non-ASCII cell (`len` mutant survives; cmd oracles use `len`). Add a non-ASCII payee case beside an ASCII one to `render_recurring_internal_test.go` and `render_anomalies_internal_test.go` with literal padded rows.

## MINOR (fold if cheap)
- correctness-B — `internal/cli/recurring.go:48,58`, `anomalies.go:42,52`: clock read twice; read once (`at := now()`) for window and request.
- correctness-A — `store.Charge.Account` partly filled (ID, Name, Currency, Closed, Active): document it.
- test-B — cmd future-dated/local-date wiring test per command (UTC-5 clock, charge dated tomorrow-local, `--until 2027` → absent, caption local date).
- test-B — cmd anomalies multi-split e2e: two-category split from a payee with ≥3 earlier → `(split)` and `payee, N earlier`; NULL+one-category first-time → not_judged.
- test-B — unknown-flag usage-hint rows for `recurring --bogus`, `anomalies --bogus` (`run_usage_test.go:199`).
- test-B — logic in test bodies (`if c.noCharges`, `if c.accounts != nil`) in `run_recurring_empty_test.go:104`, `run_anomalies_empty_test.go:93`, `run_recurring_refusals_test.go:64`, `run_anomalies_refusals_test.go:58`.
- test-B — `run_anomalies_account_test.go:88-90` `require.Len` before index; reuse `decodeAnomaliesJSON`.
- test-B — delete decorative `internal/cli/window_internal_test.go`; drop duplicate `Test_recurring_help_shows_each_flag` and anomalies root-Short duplicate.
- test-B — `internal/cli/json.go:161` unreachable reason onto one line.
- test-A — `duckstore/charges_test.go:150` `if c.transfer != nil` in t.Run; `anomalies_test.go:19-49` mixed-rule table (drop floor row, use `amountsOf`); `recurring_account_test.go:78` implicit default amount; `recurring_test.go:143-161` use `recurringOf`.
## Deferred (→ STATE.md Open debts)
- refactor MINORs: Long text hard-codes policy numbers; JSON wire words reuse display maps; positional `cadenceRules` literal; `Server.Anomalies` compose; duplicate command consts in report; split-category rule in two mappers (`CategoryPath()`); tenths/percent constants; `steady()` placement.
- correctness-A: int64 overflow in cents arithmetic (unreachable at real amounts).
- test-A NITs: package-level fixtures; duplicate `reads == 1`; recurring non-OpenError pass-through twin; vacuous loop assert; adapter arms (transfer+expense legs, dangling payee_id).
- test-B: `recurringTable`/`anomaliesTable` oracle duplication; R2/R3 at cmd for new commands.

## Verdict: BLOCKED
