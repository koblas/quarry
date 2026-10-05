---
id: SCENARIO-14a
status: open
---

# SCENARIO-14a: Net worth warns about unpriced and unconvertible holdings

Cadence: code-first (no mandatory test-first item: no guard on user files, no atomic adapter, not a bug fix)
Acceptance test: `cmd/quarry/run_networth_holdings_warnings_test.go` `Test_run_networth_warns_about_each_holding_it_leaves_out_and_accounts_gives_the_same_lines`
Narrow loop: `go test ./internal/store/duckstore/ -run 'Unvalued|NetWorth|Accounts|Faults' && go test ./internal/report/... ./internal/cli/ -run 'NetWorth|Accounts|Unvalued|Holdings' && go test ./cmd/quarry/ -run 'networth|accounts'`
Mutation checks: shared left-out predicate in `unvaluedHoldingsSQL` → `Test_unvalued_holdings_match_the_view_count_per_account_and_day`; counted-accounts filter in the networth read → `Test_run_networth_leaves_a_not_in_reports_accounts_unpriced_holding_out_of_its_warnings`; listed-accounts filter in `(*Server).Accounts` → `Test_accounts_warns_only_about_listed_accounts_holdings`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`internal/report` + `document`; `store`/`duckstore` are its port and adapter, `cli` and `cmd/quarry` wiring); no new port method

Inputs read: STATE.md only (no SCENARIO-NN.md opened). Existence found by `go doc`, LSP-less direct Read of the ranges below.

## Decisions (orchestrator review: items 1 and 2)
1. **Networth warns only for counted accounts** (N-7): the read filters on `reportedAccount`, so a not-in-reports or linked-tracking account's unpriced holding never warns there; `quarry accounts` lists every account it shows, so it warns for each *listed* account (closed ones only with `--all`; `Server.Accounts` filters the rows with the accounts, so no warning names a hidden account).
2. **Accounts order: `allClosedNote`, then the new holdings lines, then `accountsFXWarnings`** (the ruled networth order puts rate warning last). No pinned order forbids it: no existing test has a holdings warning beside an FX one.
3. Facts = new rows, not a counter: `store.UnvaluedHolding` list on `store.NetWorth` and `store.AccountList`, read in the same open (one read per command). `HoldingsUnvalued` on `AccountBalance` is **not built**; rows give the per-account count.
4. Classification in the composer from the row: unpriced = no price (whatever its currency; holdings precedent); no currency = NULL currency (priced or not); other currency = priced, currency not CAD/USD (`report.Convertible` rule, shown in every mode, native too). A priced CAD/USD holding left out only for lack of a rate gets none of 3-5 (14b owns rate lines).
5. History wording, verbatim: `holds a security with no price on N of the month ends listed` where N = listed dates on which that account had an unpriced holding (N=1 reads `on 1 of the month ends listed`: flag for copy ruling, not invented). No-currency and other-currency lines appear once per account x security if the condition holds on any listed date.
6. Within a kind: account name ignoring case, then name, account id; no-currency/other-currency then security name ignoring case, then name, security id. A security with no name is called by its id (holdings precedent).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_networth_holdings_warnings_test.go` `Test_run_networth_warns_about_each_holding_it_leaves_out_and_accounts_gives_the_same_lines` — one fixture (past clock `holdingsClock()`; mirror `seedHoldingsStoreWithUnpricedHolding` at `run_holdings_no_price_test.go:28-40`): a brokerage with an unpriced holding, a no-currency security and a EUR security; runs `networth` then `accounts`; asserts the three ruled lines verbatim on stderr (prefix `quarry: warning: `), exit 0, and `accounts` stderr holds the same three lines
- [x] Step 2: `internal/store/store.go:197-205,643-649` `UnvaluedHolding` + `NetWorth.Unvalued` + `AccountList.Unvalued` — signature-only; `report.NetWorth.Unvalued`; `document.NetWorthWarnings` / `document.AccountsWarnings` stubs returning nil so the test reddens at its assertion. Fixtures (`report/fakes_test.go:122`, `cli/fakes_test.go:38,102`) need no change: fields only

### Build
- [ ] Step 3 (batch 1, adapter): `duckstore/schema.go:305-330` extract the `valued` CTE's CASE into `valuedInAccountCurrencySQL()` (behaviour-neutral); `duckstore/networth.go:26-55` second `QueryRows` in the same open: v_holdings x accounts on `date IN (...)`, `reportedAccount`, investment types, value in account currency NULL, ordered; `duckstore/accounts.go:24-55` the same at `current_date`, no counted filter. Tests: `duckstore/unvalued_holdings_test.go` — rows for each arm (unpriced, NULL currency, EUR priced, priced with no rate), `Test_unvalued_holdings_match_the_view_count_per_account_and_day` (per account x date equals `v_balances_daily.holdings_unvalued`, with a valued control), not-in-reports/linked-tracking absent from NetWorth but present in Accounts, past-64-bit unaffected; fault tests: `read_faults_test.go:22-60` rows need a query fault and a scan fault on the **second** query only (extend `spyReadDB` if it lacks fault-on-Nth) for NetWorth and Accounts; empty `Dates` still returns no rows without the query
- [ ] Step 4 (batch 2, report): `report/networth.go:57-85` carry `read.Unvalued` filtered to requested days (as rows are); `report/accounts.go:48-67` `Accounts` filters `Unvalued` to the accounts it lists (closed ones dropped unless `includeClosed`). Tests: `report/networth_test.go` (row on an unrequested day dropped; every request reads the store once), `report/accounts_test.go` `Test_accounts_warns_only_about_listed_accounts_holdings` (hidden closed account's holding dropped, `includeClosed` keeps it)
- [ ] Step 5 (batch 3, composer): new `report/document/holdings_left_out.go` — `NetWorthWarnings(n report.NetWorth) []string` (history iff `n.Window != nil`; later 16 puts its empty line before and 14b its rate lines after, in this one func) and `AccountsWarnings(l report.AccountListing) []string` (as-of = `l.AsOf`), both over one unexported builder; sibling of `HoldingsWarnings` (`holdings.go:109-119`), reuses `perSecurity`'s name-or-id rule, `report.Convertible` shape and `humanize.Count`. Tests `holdings_left_out_test.go` (builder, rows asserted verbatim): snapshot singular and plural; history N and the per-account date count (1 and >1 dates); each kind in its own row and a no-price-no-currency holding gives both 3 and 4; priced CAD/USD-no-rate gives none; ordering by account name ignoring case with an upper-case-first control, tie by name then id, kinds in ruled order across two accounts, security order within an account; same security in two accounts warns for each; nil/empty rows give `[]` not nil; `%q` on a name with a quote
- [ ] Step 6 (batch 4, wiring): `cli/networth.go:262-269` `warnings := document.NetWorthWarnings(netWorth)` into `emitReport` and `withConfigWarnings(configWarnings, warnings)`; `cli/accounts.go:319-323` insert `document.AccountsWarnings(listing)` between `allClosedNote` and `accountsFXWarnings`. Tests, one pin per site and format: `cmd/quarry/run_networth_holdings_warnings_test.go` snapshot text, `--json` `warnings[]` (config warning first, holdings after), history text with the "N of the month ends" line, `--currency native` (EUR still warns), `--as-of` before the price (unpriced), counted-only `Test_run_networth_leaves_a_not_in_reports_accounts_unpriced_holding_out_of_its_warnings`; `cmd/quarry/run_accounts_investment_balance_test.go` pin for `accounts` text and `--json` `warnings[]` order beside an FX warning, `accounts --all` vs default for a closed account's holding; `internal/cli/networth_test.go`/`accounts_test.go` fake-store row proving the lines reach stderr only after a successful stdout write (`emit` order)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (budget: exported ~4 lines) on `UnvaluedHolding`, both composers; fix cmd goldens that now carry a warning (an existing fixture with an unpriced or NULL-currency holding was silent until now); update `store.go`/`json_accounts.go` docs that still say holdings left out silently, if any

### Verify
- [ ] Step 8: full verification per `.claude/rules/agent-briefs.md` + `spec-check.py phase4c-networth`; tick SCENARIO-14a with its acceptance test; time `quarry accounts` and `networth` on the synthetic store (Accounts now also scans v_holdings at today, and every `--account` resolve pays it; report ms next to S07's 70 ms); rewrite STATE.md (drop the three "Left unbuilt" 14a bullets)

## Handoff

**Binding decisions:**
- Left-out holdings travel as `store.UnvaluedHolding` rows on `store.NetWorth` and `store.AccountList`, never a counter — S14b/S16 add their facts (first rate, first transaction) as more fields in the same open; the networth text/JSON warnings come from `document.NetWorthWarnings`, which S16 (empty line, before) and S14b (rate lines, after) extend in place so the ruled order stays in one func.
- Left-out predicate lives once in `valuedInAccountCurrencySQL()`, shared by `v_balances_daily` and the unvalued reads — S06's `holdings_unvalued` and these rows cannot disagree.
- Networth warns for counted accounts only; accounts warns for listed accounts only (decisions 1-2).

**Left unbuilt:**
- `HoldingsUnvalued` on `AccountBalance` — not needed, rows suffice.
- Rate warning 6, `no rate` history cell, `Total <CUR>` — S14b; empty-result warnings — S16.

**Traps:**
- `store.Accounts` also serves `resolveAccounts` (every `--account` command): the second query adds cost there; step 8 times it.
- v_holdings carries a NULL-price row until a price exists; "priced zero" counts as priced (`Price != nil`), as `holdings` does.
- Warnings 3-5 never fire for a priced holding unvalued only because no rate exists; do not widen the classifier into 14b's lines.

## Phase report

Run A (steps 1-2) done; acceptance red.

Files:
- `cmd/quarry/run_networth_holdings_warnings_test.go` — acceptance test + `seedLeftOutHoldingsStore`, consts `leftOutNoPriceLine`, `leftOutNoCurrencyLine`, `leftOutOtherCurrencyLine` (reuse in B4 pins).
- `internal/store/store.go` — `UnvaluedHolding{Date, AccountID, Account, SecurityID, Security string, Currency *string, Priced bool}`, `NetWorth.Unvalued`, `AccountList.Unvalued` (zero-value, nothing fills them yet).
- `internal/report/networth.go` — `NetWorth.Unvalued []store.UnvaluedHolding`.
- `internal/report/document/holdings_left_out.go` — `NetWorthWarnings` / `AccountsWarnings` stubs returning nil (doc says never nil: B3 makes it so).

Red (quoted): networth `Not equal: expected: "quarry: warning: \"Brokerage\" holds 1 security with no price on or before 2026-03-12, ...three lines" actual: ""`; then `require.Len` on accounts stderr lines: `"[]" should have 3 item(s), but has 1`.

Decisions B runs must keep:
- Bare Fund in the fixture has NO price row at all (not "price after the clock"): `accounts` reads the store's real today, so a price dated 2026-03-13 would be valued there and drop its line. Networth line says `on or before 2026-03-12`; accounts line date is the real today, so the test matches it with `\d{4}-\d{2}-\d{2}`, not a literal.
- Expected order in both commands: no price, no currency, other currency; accounts stderr has exactly these three lines (no other warning fires on this fixture).
- `Priced bool` (not `Price *int64`) is the classifier input: unpriced = !Priced; no currency = Currency nil; other currency = Priced and Currency not CAD/USD. B1 may change field shape if the scan wants it; update stubs' users then.
