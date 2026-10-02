---
id: SCENARIO-19
status: done
---

# SCENARIO-19: Accounts show each balance in the reporting currency, never a total (folds SCENARIO-11)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_accounts_fx_test.go` `Test_run_accounts_shows_each_balance_in_the_reporting_currency`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_currency_native_test.go` `Test_run_currency_native_reproduces_the_pre_fx_output` (5 rows: spend, cashflow, recurring, anomalies, accounts)
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)account|native|help|fx_warning'`
Mutation checks: view cutoff `current_date` local → `Test_accounts_use_the_local_date_in_every_zone` (mutate the view to the UTC date); blank vs `no rate` (`Balance == nil`) → `Test_renderAccounts_leaves_a_not_imported_cell_blank_and_says_no_rate_for_a_missing_rate`; no-rates warning guard → `Test_accounts_warn_of_no_rates_only_when_a_balance_needed_one`; native omits the column and the cells → `Test_run_currency_native_reproduces_the_pre_fx_output` (make one command pass `money.CAD` for native, confirm its row reddens)
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 Build batches + the folded 11 outline in Acceptance, 1 feature package (`report`; `duckstore` + `store` port fields) + cli. Folds 11 (test only: 08/10/17/18 built native; of the five rows only accounts `--json` is red, since `cli/accounts.go:15` still discards the currency and accounts text is native today).

Survey. Exists, reused: `v_account_balances.balance_cad/usd` (`duckstore/schema.go:143-168`, `convertedTo`, latest rate <= `current_date`; same-currency is identity, so a CAD account in CAD converts with no rates); `accountsCurrencyHelp` bound at `cli/accounts.go:52` and pinned (`report_help_test.go:174`); `currencyFlag.resolve` value discarded with `_` at `cli/accounts.go:15`; `noRatesWarning` (`fx_warning.go:12`); `withConfigWarnings`; `topLevelKeys` (`json_spend_internal_test.go:177`); `replaceStoreWithRates` (`cmd/quarry/run_sql_fx_test.go:32`); `accountsReads` on the report fake (`report/fakes_test.go:24`). Callers of `(*report.Server).Accounts` (grep, LSP does not follow the receiver): `cli/accounts.go:31`, `report/accounts_test.go:39,56,81,92`, `refusal_test.go:92,150`.
`current_date` debt, probed: DuckDB's `current_date` follows the process zone (TZ=America/Toronto gave the 1st while UTC, Auckland and Kiritimati gave the 2nd at 00:56 UTC; `current_setting('TimeZone')` echoes TZ). The assumption holds, so the view STAYS `current_date` (`quarry sql` users read it, Long says "today's rate") and the debt closes with a pin, not a fix. In-process `Setenv("TZ")` is not trusted (ICU caches the zone); re-exec the test binary.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_accounts_fx_test.go` (new) `Test_run_accounts_shows_each_balance_in_the_reporting_currency` — `replaceStoreWithRates`: CAD, USD and brokerage (not imported) accounts, a past rate plus a 2099 rate (`current_date` cannot move). Text: header `Account  Type  Currency  Balance  In CAD  Status`, USD cell converted at the past rate, CAD cell identity, brokerage cell blank, no Total line, stderr empty. `--json`: top-level `currency` "CAD", rows keep native `balance`, `converted_balance` set / null (brokerage). Same store `--currency USD` (header `In USD`).
- [x] Step 2: signature-only stubs — `store/store.go:453-465` `AccountBalance.BalanceCAD, BalanceUSD *int64` and `AccountList.FirstRate time.Time` (min fx_rates.date, zero = no rates); `report/accounts.go:28` `Accounts(ctx, includeClosed, currency money.Currency)` + `AccountListing.Currency`; `cli/accounts.go:15,31` keeps `resolve`'s value and passes it. Red at the header assertion.

- [x] Step 3: `cmd/quarry/run_currency_native_test.go` (new) `Test_run_currency_native_reproduces_the_pre_fx_output` — five commands on ONE CAD+USD store seeded with rates, so it guards native for every later batch (it is in the narrow loop, so a padding or trailing-space slip in Step 5 reddens at once). ORACLE: literal text captured from the pre-2f tree (`git archive 6908035 | tar -x -C $TMPDIR/pre`, the parent of spec commit `71ca07d`; `go.mod` is identical to HEAD's, so it builds offline), by a throwaway test there using THAT tree's `store.Rows`/`store.Account` shapes; paste the strings, never regenerate them from this tree (a regression shared by both would pass a self-comparison). Fallbacks only if the capture is blocked: existing pre-change native literals (`run_spend_fx_test.go:108` `nativeText`, the "literal pre-18 text" row in `run_anomalies_fx_edges_test.go`) for spend, cashflow, anomalies. Clock: `run()` uses `time.Now`, and recurring's active/ended state and default windows read `Env.Now` (`cli/recurring.go:58`), so the data must make output independent of today (explicit `--since/--until`, series that ended years ago, all dates in the past) or be built from dates relative to now; keep accounts `--json` `as_of` out of any literal. Asserts: text equals the literal, stderr empty, same text on an unrated copy (rates never leak into native). `--json`: `currency` "native"; recurring `native_*` == `currency/amount/first_amount`; anomalies `native_*` == `currency/amount/usual`; accounts `converted_balance` null on every row; spend and cashflow `currency` only. Red now only at the accounts `--json` assertion; the four other rows are green on arrival because 08/10/17/18 built them, and accounts text is native today.

### Build
- [x] Step 4: `duckstore/accounts.go:11-17,31-57` `accountsQuery`/`Accounts` — read `v.balance_cad/usd` as cents and `(SELECT min(date) FROM fx_rates)` in the SAME statement (no second read); `report/accounts.go:28-45` — `Currency` echoed; `(AccountListing).ConvertedBalance(a) *int64` (native, not-imported, no-rate all nil) and `NeedsRate(a)` (imported, Currency != Native, cell nil).
  - duckstore tests (`accounts_test.go`): CAD in CAD identity with no rates; USD no rates nil; investment nil in both; USD converted at the latest rate <= today ignoring a 2099 rate; CAD to USD; `FirstRate` zero / set; NULL-cell scan.
  - `Test_accounts_use_the_local_date_in_every_zone` (re-exec `os.Args[0]` with TZ=Pacific/Kiritimati and TZ=Pacific/Pago_Pago, +14 and -11: their dates always differ, so one of them differs from UTC at any instant): seed a rate dated the Go-local today and a different one dated Go-local tomorrow; assert `AsOf` and the converted cell use the local today. Closes the STATE.md debt.
  - report tests: each currency picks its cell; native nil; echoed; `accountsReads == 1`; fault test unchanged (`refusal_test.go:92`).
- [x] Step 5: `cli/render_accounts.go:18-43` `renderAccounts(report.AccountListing)`, `cli/fx_warning.go` `accountsFXWarnings`, `cli/accounts.go:30-47` — the column only when `Currency != Native`, after Balance, header `In <CAD|USD>`, right-aligned, blank / `no rate`; the line is right-trimmed (a blank last cell leaves no trailing spaces; native has none, so it stays byte-identical). No Total anywhere. Warning `noRatesWarning` when `FirstRate` is zero AND some row `NeedsRate`; order config, all-closed note, FX (the first two and an FX line cannot coexist: all-closed means no rows).
  - Tests: `render_accounts_internal_test.go` (8 call sites rewrap in a zero-Currency `AccountListing`, so every existing native want stays literal): CAD and USD headers, blank vs `no rate`, widths with a wide `no rate`, status after the column, header-only (all closed) keeps the column, trailing-space pin, closed account converts under `--all`. `fx_warning_internal_test.go` + cli: warns on unrated USD; silent on an all-CAD unrated store in CAD; silent when native; rated store silent; rates only after today = `no rate` cells with no warning (n/a otherwise: `FirstRate` set); one USD `--account`-style cross is n/a (accounts has no filter).
  - Repoint (default CAD now adds the column; keep native goldens by adding `--currency native` where the test is not about the column): `cmd/quarry/run_accounts_test.go:18-231` (6 goldens, headers at :29,48,151,166,199,231), `run_accounts_json_test.go:38-150`.
- [x] Step 6: `cli/json_accounts.go:7-43` — top-level `currency` after `as_of`, row `converted_balance` after `balance`, null in native / not imported / no rate. `cli/accounts.go:11-17` Long appended with the ruled paragraph (wrapped at 72 columns, words verbatim). Tests: key-ORDER pin with `topLevelKeys` (top level and a row, CAD, USD, native: identical key set), null forms, read-back with `encoding/json` (row count, balances, `accounts []` for none, `warnings` incl. the FX line), `json_accounts_internal_test.go:14-120` rewrapped, Long verbatim in `run_accounts_test.go:84-140`.

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`AccountList.FirstRate`, `ConvertedBalance`); bump any exact-count pin (`test-stats`).

### Verify
- [x] Step 8: full verification + `spec-check.py phase2f-fx`; tick SCENARIO-19 and SCENARIO-11 ("delivered by SCENARIO-19", test last on the line); rewrite STATE.md (delete the `current_date` debt and the 19/11 Left-unbuilt row).

## Handoff

**Binding decisions:**
- View stays `current_date`; zone pinned by re-exec test — `quarry sql` readers and the Long ("today's rate") depend on it.
- `store.AccountList.FirstRate` rides the one `Accounts` call — one read per command; the warning needs "store has no rates", which `NULL` cells alone cannot tell from future-only rates.
- `AccountBalance.BalanceCAD/USD` are cents, nil = NULL; `report.AccountListing.Currency` zero = Native renders today's output (fixtures stay valid).
- The 11 oracle is literal pre-2f text from `6908035`, not a self-comparison.

**Unruled copy (chosen here, product-vision may overrule at the final pass):**
- `currency` JSON key sits after `as_of` (accounts has no `until`); `converted_balance` is the last row key.
- In CAD cell right-aligned; `no rate` right-aligned; trailing spaces trimmed.
- Some accounts convert and others do not (unrated store, CAD accounts identity): the one ruled no-rates line, nothing per account. Its wording ("amounts are listed in each account's own currency") fits accounts loosely.
- Rates all dated after today: `no rate` cells, no warning.
- Header-only (all closed) in CAD mode keeps the `In CAD` column; USD mode header `In USD` is ruled only via "`In CAD`/`In USD`".
- Long wrap: 72 columns, ruled words verbatim.

**Route before run A:** the unruled list above goes to a scoped `product-vision` copy ruling (CLAUDE.md, rule copy when a new outcome appears), not to the final gate.

**Left unbuilt:** REFERENCE-CHECK (SCENARIO-20); no total, no `--account` on accounts.

**Traps:**
- Default CAD now adds the column to every accounts golden on an unrated `replaceStore`: add `--currency native`, do not seed rates.
- `current_date` cannot be moved in-process; a rate dated 2099 proves "ignore later rates", the re-exec test proves the zone.

## Phase report

Run V done: Sweep (doc comments trimmed to budget in `duckstore/accounts.go`; lint 0 issues), covered full suite rc=0, uncovered-diff 0 lines, race green on duckstore, report, cli, cmd/quarry. Steps 7-8 ticked, SCENARIO-19 and 11 ticked, spec-check OK, STATE.md rewritten, status done. Nothing left for later runs.
