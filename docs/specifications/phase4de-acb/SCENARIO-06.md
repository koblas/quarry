---
id: SCENARIO-06
status: open
---

# SCENARIO-06: Sync imports Quicken's cost basis

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_cost_basis_test.go` `Test_run_sync_keeps_quickens_cost_basis_and_stores_null_for_none`
Narrow loop: `go test ./cmd/quarry/ -run 'CostBasis|FormatVersion|SQLConventions|schema_reference|shared_documents' && go test ./internal/importer/ ./internal/store/... ./internal/report/ ./internal/cli/ -run 'CostBasis|cost_basis|Replace|Conventions|Sql|refuses_an_investment'`
Mutation checks: none (code-first; the NULL-when-zero arm and the commission-NULL early return are pinned by named rows in Step 4)
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches; importer + store/duckstore (+ report const); sizing-pass verdict kept (mechanical column carry, `commission` is the precedent)

**BLOCKS DISPATCH of run A: (1) copy ruling on the new `cost basis` refusal (Step 4); (2) P2b ruling (Handoff) — a ZORDERID ordering column would have to land in this S06 format bump.**

Surface surveyed (grep; LSP `findReferences` on `Commission` sees only its declaration, cross-package unreliable): the one writer is `duckstore.investmentTransactionRows`, the one producer `importer.readValues`; no duckstore reader builds `InvestmentTransaction` from rows. No new port: `Store.Replace(Rows)` unchanged. `status.go:25`, `history.go:44`, `document/common.go:34`, `holdings.go:72`, `shares.go:31` (names its columns) need no change.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_cost_basis_test.go` (new) `Test_run_sync_keeps_quickens_cost_basis_and_stores_null_for_none` — mirror `run_commission_test.go:17-47`. Rows: buy with cost, reinvest (amount 0) with cost, add_shares with cost, add_shares without, sell with ZCOSTBASIS `0` (NULL), dividend (NULL). Assert `format_version` is the literal `"9"` first via `assert` (genuine red), then `cost_basis` per id via `stringMap`
- [ ] Step 2: stubs so it compiles and runs. `v9fixture/builder.go:65-86` `TransactionRow.CostBasis` + INSERT column at `:514-524` + typeof/NULL asserts at `builder_test.go:166-191` (defaults NULL; no existing fixture changes, so STATE's no-shared-helper rule holds); `store/store.go:277-291` `CostBasis *int64` (cents) after `Commission`; `duckstore/schema.go:89-103` `cost_basis DECIMAL(18,2)` directly after `commission`; `duckstore.go:674-677` cell in the same slot (`appendTable` matches by POSITION). Red = values NULL and format `8`

### Build
- [ ] Step 3 (B1, store): `duckstore.go:24-25` `FormatVersion = 9`; `:663-680` `decimalCell("cost_basis", t.CostBasis, moneyWidth, moneyScale)` joined into the error set. Tests `duckstore_test.go`: `minimalRows` `:59-70` (inv-1 set, inv-2 nil), round-trip `:129-153` set and NULL, largest value `:495-511` (`9999999999999999.99`, the in-bound control), out-of-range row in the table `:512-535` (`beyond18Digits`, error names `cost_basis`). Re-pin literal `8`→`9`: `run_holding_shares_test.go:56`, `run_investment_cash_test.go:90`, `run_shared_documents_test.go:134`. Constant-based pins (`open_test.go:180` older-format = 8, `status_test.go:36`, `run_store_info_test.go:73`) need nothing
- [ ] Step 4 (B1, importer): `investments.go:47-56` query adds `typeof(t.ZCOSTBASIS), CAST(t.ZCOSTBASIS AS TEXT)` (scan order at `:122-125` matches); `:96-106` `investmentRow.costBasis`; `:267-295` `readValues` — the `return true` at `:284-286` on a NULL commission must not skip the cost read: split commission and cost basis into helpers. Cost basis: NULL column → nil; `s.money(col, "a cost basis", amountColumn)` (`:331`, `amountColumn` `:325`); 0 → nil; negative stored as recorded. First fault wins: shares, amount, commission, cost basis. Add "a cost basis" to the `what` list at `reasons.go:142` and the class comment `offenders.go:20`. Tests: `investments_test.go:328` `Test_import_stores_cost_basis_in_cents_and_null_for_none_or_zero` (rows NULL, `0`, residue `0.000000001`→NULL, `1000.50`, integer-typed `1000`, `-5.00` as recorded, commission NULL with cost set, commission set with cost NULL); refusal rows in `:407-444` (more than 2 decimals `1.234`, too large `10000000000000000`, not a number `n/a`; each reached through the real decode; one row with commission AND cost both unreadable asserts the commission message); one command-slice row beside `run_investments_test.go:260`. Blob: n/a, same `parseMoney` default arm as the text row. Output-format cross: n/a, sync prints no cost column

### Conventions copy (B2)
- [ ] Step 5: `report/sql_conventions.go:24-33` insert sentence 1 VERBATIM, mid-paragraph right after `so a sum of shares is not a holding.` and before `prices holds`: `cost_basis is the cost Quicken records for a buy, reinvested dividend or added shares (NULL when none).` Sentence 2 (`ACB and capital gains are in no table or view: quarry acb (MCP acb) computes them; never derive them in SQL.`) is S19's: it names a command that does not exist yet, so nothing here says `acb`. Hand-wrap to the const's width. Test: verbatim pin in `report/sql_conventions_test.go` (whitespace-collapsed, as `:27-37`); `Test_sql_conventions_close_the_investment_paragraph_with_the_action_values` must stay green. Re-copy the paragraph into `internal/cli/sql_test.go:222-242` and `cmd/quarry/run_shared_documents_test.go:366-385`; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update` (adds the `cost_basis` row and the sentence; never hand-edit)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `CostBasis` (cents, NULL when Quicken records none) and the `InvestmentTransaction` unit line `store.go:272-276`

### Verify
- [ ] Step 7: full verification block + `.claude/scripts/spec-check.py phase4de-acb`; tick SCENARIO-06 with its acceptance test; rewrite STATE.md (move the `cost_basis` line out of Left unbuilt; add the P1/P2 results and Binding decisions below)

## Handoff

**Orchestrator rulings 2026-10-05 (product-vision; recorded in spec):** (1) same-day order = acquisitions, then splits/adjustments, then dispositions, ties by source_id — NO ZORDERID column, format-9 bump carries only cost_basis; (2) cost-basis refusal copy confirmed as planned (what = "a cost basis"; extend the comment at internal/importer/reasons.go:142); (3) pairing does not ship; SCENARIO-10 rewritten (user-approved) as "Shares added or removed without a trade", LIGHT.


**Probe results (snapshot 20261004T184923Z; counts only; no price column exists on ZTRANSACTION)**
- **P1 sell commission: NET.** Quote method (units x closing price, +-1 day, 24 brokerage sells with commission) cannot separate them: commission is 0.002-0.375% of amount, inside the 0.5% tolerance; at 0.01/share it gave 0 net-only, 1 gross-only, 1 both, 18 neither. Price-grid method decides: a fill price sits on the cent grid, so test whether (amount + commission)/units (net) or amount/units (gross) lies on it within 0.005/units. Control, buys with commission (commission inside amount): 13 of 14 fit, gross 0. Sells with units >= 50 (20): net 6 on a 0.01 grid; widening to 0.001 for units >= 300 adds 1 (7 net in all); **gross 0; mixed 0; undecided 17** (14 off the cent grid, 4 under 50 units). The one quote-method gross-only row is net on the grid. Conclusion: net, no row supports gross, so no ruling; 08a keeps proceeds = amount + commission.
- **P2a pool-internal pairs: 0** (add vs remove, same security, same day, equal and opposite, different brokerage accounts; also 0 same-account and 0 within +-3 days). 25 add_shares and 1 remove_shares carry units, none paired. Per spec line 199 pairing does NOT ship: **binding for 08a and 13b** — every add/remove is unpaired, no `moved` row, and shares-without-cost's "not a pool-internal move" clause is dead.
- **P2b same-day buy+sell: 2 days (4 buy/sell pairs, all one account), INVERTED.** By Z_PK a sell precedes the buy in 3 of 4 pairs, on 2 of 2 days. ZORDERID and ZCREATIONTIMESTAMP put the buy first or equal in 4 of 4 (sell before buy 0); ZENTEREDDATE is equal in all 4. So `source_id` order disagrees with Quicken's own order key. **Needs a product-vision ruling BEFORE run A**: spec line 200's fallback ("acquisitions before dispositions on the same day", needs no column) versus ordering by ZORDERID (needs a new column in this S06 format 9 bump).
- P3: ZCOSTBASIS on 1611 txns: integer 1345 (all 0), real 266; non-zero 278 (buy 272, reinvest 5, add_shares 1); none negative, none beyond the cent snap tolerance. A refusal cannot fire on the real file.

**Binding decisions**
- `cost_basis DECIMAL(18,2)` NULL sits directly after `commission` in the DDL and in the `investmentTransactionRows` slot order — `appendTable` is positional. Store field `CostBasis *int64` in cents.
- A refusal (not NULL) for an unreadable ZCOSTBASIS; the copy is the existing `reasonInvestment*` templates with `what = "a cost basis"` — needs the orchestrator's scoped copy ruling, spec has none.
- Conventions: S06 ships sentence 1 only; S19 inserts sentence 2 immediately after it, in the same mid-paragraph spot, never at the end (action-list suffix test) or as a new paragraph (tests index paragraphs).

**Left unbuilt**
- A duckstore/report reader of `cost_basis`, and 13b's `FindingList` input for shares-without-cost — 08a/13b build their own reads through SQL or a new port method.
- A ZORDERID column — only if the P2b ruling asks for it.

**Traps**
- `readValues` returns early on a NULL commission (`investments.go:284-286`); a cost read appended below it never runs for the common no-commission buy.
- `run_sync_pre4a_store_test.go:49` "8"/"9" are import-run ids, not format versions; leave alone.
- Hand copies of the conventions wrap by hand; a reflow in the const that is not copied byte-for-byte fails `cli/sql_test.go` and `run_shared_documents_test.go`, and `schema.md` is regenerated, not edited.
