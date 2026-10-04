---
id: SCENARIO-10
status: open
---

# SCENARIO-10: Filtering by account

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_holdings_account_test.go` `Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing`
Narrow loop: `go test ./internal/report/... ./internal/store/duckstore/ ./internal/cli/ -run 'Holdings|Account'` and `go test ./cmd/quarry/ -run 'run_holdings'`
Mutation checks: `IN` clause in `holdingsQueryFor` → `Test_holdings_reads_only_the_named_accounts`; `IsInvestmentAccount` predicate in `nonInvestmentWarnings` → `Test_HoldingsWarnings_name_each_named_account_that_is_not_an_investment_account`; `NewAccountFilters(h.Accounts)` reverted to `NewAccountFilters(nil)` in `NewHoldings` → `Test_holdings_json_account_filter_lists_the_named_accounts`; table-key sort in `nonInvestmentWarnings` removed → its Zeta-before-Alpha row
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/report` + store port/adapter + cli wiring)

Surveyed (grep, production only): `store.HoldingsParams` is built in one place, `report/holdings.go:65`; `report.Store.Holdings` has two test fakes (`internal/report/fakes_test.go:96`, `internal/cli/fakes_test.go`) and one adapter (`duckstore/holdings.go:26`); a new field breaks none. Resolution and refusal exist: `(*Server).namedAccounts` (`report/accounts.go:92`) → `resolveAccounts` (`:70`: id first, else name ignoring case, repeats dropped, closed accounts resolvable) → `unknownAccountRefusal` (`report/refusal.go:86`, exit 1 as spend). Also `NewAccountFilters` (`document/account_filter.go`), `accountsCaption` (`cli/render_table.go:64`), `accountFilter.marks` (`duckstore/filter.go:40`). `accountFilter.and` numbers from `$3`: do not use it (holdings has `$1` = day, ids start at `$2`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_account_test.go` `Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing` — `holdingsRows()` (`run_holdings_test.go:51`) plus `chequingAccount("acct-chq", 4)`; `--account Brokerage --account Chequing`: stdout caption `Holdings on 2026-03-12 in Brokerage, Chequing, amounts in CAD; cash not included`, the Acme line and Total only, built with the file's OWN line helper sized to the filtered fixture (`holdingsLine`, `run_holdings_test.go:16-25`, is sized to the 3-account table and misaligns); stderr exactly the S.3 non-investment line with `quarry: warning: ` prefix; exit 0
- [x] Step 2: signature-only stubs so it compiles: `store.HoldingsParams.AccountIDs`, `report.HoldingsRequest.Accounts`, `report.Holdings.Accounts`, `--account` flag registered

### Build
- [x] Step 3: filter reaches the reader — `store/store.go:159-162` `AccountIDs []string` (empty reads every account); `duckstore/holdings.go:13-24,34` const → `holdingsQueryFor(accountFilter)` adding `AND v.account_id IN (<marks(2)>)`, args `civilDay` then ids; `report/holdings.go:12-16,27-35,64-71` `HoldingsRequest.Accounts []string`, `Holdings.Accounts []store.Account`, `Server.Holdings` calls `namedAccounts(ctx, "holdings", req.Accounts)` first and passes ids in the one `store.Holdings` call; no names → no `store.Accounts` read, `AccountIDs` nil (the `internal/cli/holdings_test.go:~160` equality pin stays true). Tests: `duckstore/holdings_account_test.go` (new) `Test_holdings_reads_only_the_named_accounts` (one, two, none = all, an id naming no account = empty not error, closed account listed, ids in another order keep table order); `report/holdings_account_test.go` (new): names resolve in the order given (name, id, name ignoring case), same account by name and id is one entry, unknown and empty arg → `RefusalUnknownAccount` with no `Holdings` read (`holdingsReads`), ambiguous name → `RefusalAmbiguousAccount`, `store.Accounts` fault → `readRefusal`, no names → `accountsReads` zero and `AccountIDs` nil, one-read fake whose second `Holdings` call returns other rows
- [x] Step 4: composers and caption — `document/holdings.go:84` `NewAccountFilters(h.Accounts)`; `:109-119` `HoldingsWarnings` appends new `nonInvestmentWarnings(h)` FIRST (slot 2, before `noPriceWarning`): `account %q is not a brokerage or retirement account, so it has no holdings` per `h.Accounts` entry failing `store.IsInvestmentAccount(Type)`, SORTED by the table key (plain name, `SourceID`, `ID`: R3 "several of one kind follow the table sort"; copy the key from `holdingsQuery`'s `ORDER BY`), on a copy so `h.Accounts` keeps the order given; keyed on type alone, never rows, closed, `NotInReports` or `LinkedTracking`; doc comment slot list → contract only. `cli/render_holdings.go:72` `accountsCaption(h.Accounts)` (order given). Tests: `document/holdings_account_test.go` (new) `Test_HoldingsWarnings_name_each_named_account_that_is_not_an_investment_account` (`Zeta Chequing` named before `Alpha Savings` → Alpha line then Zeta; named brokerage and retirement with zero rows, a closed brokerage and a `NotInReports` brokerage → no line; line precedes the no-price line in a row-carrying fixture), `Test_holdings_json_account_filter_lists_the_named_accounts` (`id`, `name`, order given, `[]` when none); `internal/cli/holdings_account_test.go` (new): caption keeps the order given (Zeta before Alpha), `, amounts in CAD` and none in native, a repeat named once, a closed account listed with ` (closed)` cell and a plain caption name
- [x] Step 5: cli surface — `cli/holdings.go:14,66-67` `--account` (`StringArrayVar`, `holdingsAccountFlagHelp` const beside `holdingsAsOfFlagHelp`, S.1 text verbatim: ``list only the account with this `name` or id; repeat for more``), `Accounts` into `HoldingsRequest` at `:50`; refusals stay `&runtimeError` (exit 1; `--as-of` parse still first). Tests: `internal/cli/holdings_account_test.go` help line by regexp `(?m)--account name +…$` beside `holdingsAsOfHelp` (`holdings_test.go:24`, never replacing the whole-help pin), flag value reaches `gotHoldings`; `cmd/quarry/run_holdings_account_test.go`: unknown name, empty arg, ambiguous name (two accounts called Visa) each exit 1 with the spend lines and empty stdout, text and `--json`; only non-investment named → stderr starts with the non-investment line (S09 appends the empty line after it: assert the first line, not the whole stream), stdout caption + header, no Total row, exit 0; named investment account with nothing held that day → `NotContains(stderr, "not a brokerage")` only; `--json` read-back `account_filter` `[{id,name}]` and `warnings` equal to the stderr lines in order, also with `--currency native`; `--account Brokerage --account acct-cad` lists once; no `--account` still `account_filter: []` and "all accounts"

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `HoldingsRequest.Accounts`, `Holdings.Accounts`, `HoldingsParams.AccountIDs`

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4b-holdings` → tick SCENARIO-10 with its acceptance test; STATE.md rewrite (drop `--account` from Left unbuilt)

## Handoff

**Binding decisions:**
- `report.Holdings.Accounts` (`[]store.Account`, nil when none named, order given, repeats dropped) is the one owner of the named set: caption (order given), `account_filter` (order given) and slot-2 warnings (table-key order) read it; `store.HoldingsParams.AccountIDs` is its ids — S09's empty-result composer reads `h.Accounts` for "in the named accounts" and its investment-transaction span read filters on the same `AccountIDs`, no second resolution
- Still one `store.Holdings` port call; names → ids is the existing `store.Accounts` read via `namedAccounts` (spend precedent), only when `--account` is given
- Slot 2 keys on `store.IsInvestmentAccount(Type)` alone; lines sort by the holdings table key per R3. DEVIATION for the orchestrator: spend's `leftOutWarnings` uses order given, and these accounts have no table rows; R3's sentence was applied literally
- S09 slot-in: named non-investment only → `Rows` empty → slot 2 line, then S09 appends its slot 3 empty line after `nonInvestmentWarnings` in `HoldingsWarnings` (both print, confirmed ruling); a named investment account with no holdings that day → no slot 2 line, S09's empty line only
- Permissive arms: a closed account may be named and is listed; a `NotInReports`/`LinkedTracking` investment account is listed with no left-out warning; one account by name and id, or in two casings, is one entry, one caption name, one warning

**Left unbuilt:**
- empty-result warnings (slot 3) and the investment-transaction span in the port call — SCENARIO-09; MCP `holdings` `accounts` argument — SCENARIO-13; skill and reference copy of `--account` — SCENARIO-15

**Traps:**
- `accountFilter.and` hard-codes `$3`: ids bind to the wrong parameters on holdings; use `marks(2)` (`$1` is the day)
- `holdingsQuery` is referenced only in `duckstore/holdings.go` (`:13,:34`, doc `:51`), no test: renaming to `holdingsQueryFor` touches those lines; fault tests in `read_faults_test.go` go through the real adapter
- A filtered table is narrower than the 3-account fixture: `holdingsLine` (`run_holdings_test.go:16-25`) misaligns it; `run_holdings_account_test.go` carries its own line helper
- An exact-empty stderr assertion for a named investment account with no holdings goes stale when S09 lands: assert `NotContains`
- Refusals are `RefusalError` (exit 1), not `UsageError` (exit 2)

## Phase report

Runs A (steps 1-2) and B1 (steps 3-5) done. Acceptance `Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing` green; narrow loops green; `go build ./...` and `golangci-lint run ./...` 0 issues (`golangci-lint fmt` fixed the stub's gci). Sweep (step 6) nothing left beyond doc comments, already written. No full-suite run, no `uncovered-diff.py`, no ticks or STATE.md yet: run V.

Production (all code-first):
- `internal/store/duckstore/holdings.go`: `holdingsQuery` const is now `holdingsSelect` + `holdingsOrder` consts and `holdingsQueryFor(accountFilter)` adding ` AND v.account_id IN (marks(2))`; `Holdings` binds `civilDay` then ids.
- `internal/report/holdings.go`: `Server.Holdings` calls `namedAccounts` first, one `store.Holdings` read with `AccountIDs`, `Holdings.Accounts` set; no names: no accounts read, `AccountIDs` nil, `Accounts` nil.
- `internal/report/document/holdings.go`: `nonInvestmentWarnings` (slot 2, first in `HoldingsWarnings`, sorted on a fresh slice by plain name, `SourceID`, `ID`), `NewAccountFilters(h.Accounts)`.
- `internal/cli/render_holdings.go`: `accountsCaption(h.Accounts)`; `internal/cli/holdings.go`: `Accounts: accounts` in the request.

Tests added: `duckstore/holdings_account_test.go` (5), `report/holdings_account_test.go` (6 incl. 2-row table), `document/holdings_account_test.go` (5 incl. two tables), `cli/holdings_account_test.go` (6), `cmd/quarry/run_holdings_account_test.go` (acceptance plus 6: refusal table of 6, chequing-only, empty investment account, json table of 2, name-and-id once, no account).

Deviations from the plan: the plan's "one-read fake whose second Holdings call returns other rows" is covered as `holdingsReads == 1` in the report test (the fake counts reads); the tie-break keys `SourceID` and `ID` of the slot-2 sort are not observable in the text (equal names give equal lines), so only plain-name order is pinned (including lower-case after upper-case).

Mutations, each reddened: `IN` clause disabled (`len(accounts) > 99`) -> `Test_holdings_reads_only_the_named_accounts`, `..._reads_nothing_for_an_id_that_names_no_account`, `..._reads_a_named_closed_account`; `IsInvestmentAccount` predicate inverted -> `Test_HoldingsWarnings_name_each_named_account...` (both rows), `..._leave_out_a_named_investment_account_whatever_its_state`, `..._put_the_non_investment_line_before_the_no_price_line`; `NewAccountFilters(nil)` -> `Test_holdings_json_account_filter_lists_the_named_accounts/in_the_order_given`; sort replaced by `return 0` -> both rows of the slot-2 table (Zeta-before-Alpha and the plain-name one). All reverted.

Run V to do: full covered suite + `uncovered-diff.py --profile`, `test-stats.py --base bb833a2 --changed`, `-race` on touched packages, `spec-check.py phase4b-holdings`, tick SCENARIO-10 in `specification.md` with `cmd/quarry/run_holdings_account_test.go` `Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing`, rewrite STATE.md (drop `--account` from Left unbuilt; fold this scenario's Handoff). Watch for other tests that pin the all-accounts caption, the unknown-flag table or `--help` of holdings (`--account` was unregistered per STATE.md): the narrow loops did not touch them.
