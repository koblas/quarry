---
id: SCENARIO-14
status: done
---

# SCENARIO-14: spend counts only the accounts it is given

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_spend_account_test.go` `Test_run_spend_counts_only_the_accounts_it_is_given`
Acceptance test (SCENARIO-15, folded): `cmd/quarry/run_spend_account_test.go` `Test_run_spend_warns_that_a_named_account_is_left_out_of_reports`
Acceptance test (SCENARIO-19, folded): `cmd/quarry/run_spend_account_test.go` `Test_run_spend_refuses_an_account_it_cannot_pick`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run 'Spend|spend|Spending|spending'`
Mutation checks: filter dropped from each filter site separately (spendingQueryFrom family / tag rows / tag totals / multi-tag count; one site only if all read one filtered relation) → `Test_spending_counts_only_the_named_accounts` + `Test_spending_by_tag_counts_only_the_named_accounts` + `Test_spending_by_tag_counts_multi_tag_splits_only_in_the_named_accounts`; id match moved after name match → `Test_spend_resolves_an_id_before_a_name_equal_to_it`; `EqualFold` → `==` → `Test_spend_matches_an_account_name_ignoring_case`; skip closed accounts → `Test_spend_matches_a_closed_account`; dedupe removed → `Test_spend_names_an_account_given_by_id_and_by_name_once` (asserts `Accounts` length/order, NOT rows — `IN` hides a duplicate); S6 id sort removed → `Test_spend_refuses_an_ambiguous_name_listing_its_ids_sorted`; W2 loop over argv reversed / dedupe bypassed → `Test_spend_warns_once_per_named_account_left_out_of_reports_in_the_order_given`; warnings emitted before `Spend`'s error return → `Test_spend_refuses_an_unknown_account_without_warning_about_an_excluded_one`
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (report; duckstore adapter + cli delivery)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_account_test.go` (new) — three `runWith` tests over `replaceStore(spendRows(...))` (`run_spend_test.go:20-74`): S14 = Chequing, Visa Infinite (closed), Savings, each with in-window spending, argv `spend --account chequing --account acct-<visa> --account Chequing` → caption `in Chequing, Visa Infinite`, Savings absent from rows and Total; S15 = "Old Card" `NotInReports` with spending → header-only table, stderr exactly W2, exit 0; S19 = table S5 `Chequeing` / S6 `Visa` (two Visa accounts, ids chosen so sorted order differs from insertion) → stderr exact, stdout empty, exit 1
- [x] Step 2: `internal/cli/spend.go:235-236` register `--account` via `StringArrayVar`, help verbatim from Surface & Copy; `internal/report/spending.go:13-16` `SpendRequest.Accounts []string`, `:28-37` `Spending.Accounts []store.Account` — fields only, unread; tests must fail at the stdout/stderr assertion, not on an unknown flag

### Build
- [x] Step 3: `internal/store/duckstore/spending.go:13-66` queries + `:88-124` `(*Store).Spending` — read `params.AccountIDs` (empty = every account); rows, Totals and `multiTagSplitsQuery` all filtered; new `internal/store/duckstore/spending_account_test.go`: `Test_spending_counts_only_the_named_accounts` (category + month, rows AND Totals, an unnamed in-window account present), `Test_spending_by_tag_counts_only_the_named_accounts`, `Test_spending_by_tag_counts_multi_tag_splits_only_in_the_named_accounts`, `Test_spending_with_no_named_accounts_counts_every_account` (control). Existing open/query/scan fault tests cover the call; add none unless a new statement is added
- [x] Step 4: `internal/report/spending.go:49-68` `(*Server).Spend` + new unexported resolver in `internal/report/accounts.go` taking the command name — when `req.Accounts` non-empty: `s.store.Accounts`, per arg in argv order exact id, else `strings.EqualFold` name; 0 → S5, >1 → S6 (ids sorted), first failing arg refuses; dedupe by id keeping first position; closed accounts match; `SpendingParams.AccountIDs` = every resolved id (excluded included); `Spending.Accounts` = resolved accounts. No `--account` → no Accounts read. Tests in new `internal/report/spending_account_test.go`: the mutation-check tests above plus `Test_spend_passes_every_named_account_to_the_store_in_the_order_given` (`gotSpending`, excluded id included), `Test_spend_refuses_an_unknown_account`, `Test_spend_refuses_the_first_account_it_cannot_pick`, `Test_spend_does_not_read_accounts_when_none_is_named`; fault tests: Accounts read `*store.OpenError` → R1 refusal; cancelled ctx → `spend interrupted`
- [x] Step 5: S5/S6 as `RefusalError` (nil cause) built in `internal/report/refusal.go:106-115` beside `storeRefusal`; copy verbatim; `cli` needs no change (`runtimeError` → exit 1, `cmd/quarry/run.go:137` adds `quarry: `). Test: `errors.AsType[report.RefusalError]` on both
- [x] Step 6: `internal/cli/render_spend.go:53` caption joins `Spending.Accounts` names with `, ` (else `all accounts`); `internal/cli/json_spend.go:79` `account_filter` from `Spending.Accounts` (`{id,name}`, `[]` when none); `internal/cli/spend.go:240-249` `spendWarnings` prepends W2 per `NotInReports` account via a helper taking the command name (`spend`), W2 lines before W1; `internal/cli/spend.go:220` pass `Accounts` into `SpendRequest`. Tests in new `internal/cli/spend_account_test.go`: the two W2 mutation-check tests, `Test_spend_captions_the_named_accounts`, `Test_spend_json_lists_the_named_accounts_in_account_filter`, `Test_spend_json_puts_w2_in_warnings_unprefixed_before_w1`, `Test_spend_passes_every_account_flag_to_the_report`

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `RefusalError` doc (`refusal.go:103-105`) → cause may be nil; doc comments on `SpendRequest.Accounts`, `Spending.Accounts`, resolver, W2 helper; `store.SpendingParams` doc still true

### Verify
- [x] Step 8: full verification + `spec-check.py phase2b-spending` → tick SCENARIO-14; tick SCENARIO-15 and SCENARIO-19 with "delivered by SCENARIO-14" and their acceptance tests; remove `AccountIDs`/`--account` from STATE.md Left unbuilt

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Resolver lives in `report`, reads `Store.Accounts` (carries `NotInReports`), takes the command name so `readRefusal` says `spend`/`cashflow interrupted` — SCENARIO-20 calls the same resolver, no second copy.
- S5/S6 are `report.RefusalError` with nil cause → exit 1 via `runtimeError`; not `UsageError` (they need the store, after `openReport`, P2b-13).
- `SpendingParams.AccountIDs` = every resolved id, excluded ones included; `v_spending` stays the one owner of the in_reports rule — P2b-10's spend = cashflow invariant depends on one predicate.
- `Spending.Accounts` = resolved accounts, argv order, deduped by id, excluded included; caption, `account_filter` and W2 all read it.
- `warnings[]` order: W2 (argv order) → W1 → E-lines (SCENARIO-17).
- S17 contract: "every named account excluded" = `len(Accounts) > 0` and all `NotInReports` → no E1/E2 even with zero transactions; E1a/E2a range covers only the in-report subset of `Accounts` ids.
- Copy rulings taken here (orchestrator may overrule before B1): S5 and S6 quote the argv value with `%q`; W2 and caption use the stored name; first unresolvable arg in argv order refuses; `--account ""` → S5; W1/W2 relative order.

**Left unbuilt** — named so nobody assumes it exists:
- E1/E1a/E2/E2a and the all-excluded suppression — SCENARIO-17. `cashflow --account` — SCENARIO-20.

**Traps** — things that look right and are not:
- An empty `AccountIDs` means every account: filtering excluded accounts out in `report` turns "all named excluded" into "count everything".
- `spendingQueryFrom` binds positional `?`, tag queries `$1/$2`: a filter `?` placed inside the FROM subselect textually precedes the window's `?`s and binds in the wrong order; empty list must never render `IN ()`.
- `account_id IN (...)` hides duplicate ids, so row totals cannot prove dedupe.
- `report` fakes share one `err` across Accounts and Spending: a named-account fault test fails at the Accounts read.

## Phase report

Run V (steps 7-8) done; scenario complete. Sweep was already clean: `go build ./...` ok, `golangci-lint run ./...` `0 issues`, doc comments on `SpendRequest.Accounts`, `Spending.Accounts`, `resolveAccounts`, `pickAccount`, `leftOutOfReportsWarning` present, `RefusalError` doc already allows a nil cause. Working tree clean before V, so no mutation left in `internal/report` or `internal/cli`.
- Verify: covered full suite green; `uncovered-diff.py --profile ... 2348fce` -> 0 uncovered added lines in 0 runs; `go test -race` on report, cli, duckstore green. `test-stats.py --base 2348fce --changed`: cmd/quarry 115 (+3), internal/cli 92 (+7), internal/report 52 (+12), internal/store/duckstore 146 (+7), TOTAL 405 (+29) tests, 170 (+3) tempdir, 151 (+3) disk.
- `specification.md`: SCENARIO-14 ticked, SCENARIO-15 and 19 ticked "delivered by SCENARIO-14"; `spec-check.py phase2b-spending` OK. `STATE.md` rewritten (resolver, W2 helper, S17 contract, traps; `AccountIDs`/`--account` removed from Left unbuilt).
