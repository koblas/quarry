# Specification: Phase 2d — findings and `sql --csv`

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Turn data-quality problems in the Quicken file into a cleanup worklist. Every `quarry sync` detects findings of eight types, carries their history across rebuilds (first found, fixed, reopened), and `quarry findings` lists them — each naming the exact transactions, payees or categories and the fix to make in Quicken. Findings the user has checked are ignored by listing their ids under `findings.ignore` in `config.toml`. `quarry sql --csv` exports any query.

**Out of Scope**: MCP `data_quality` tool and skill `findings.md` (Phase 3); ACB / investment findings (Phase 4); a command that writes ignore decisions (quarry never writes `config.toml`); config keys for thresholds (`findings.match_days`, `findings.mixed_min` are named policy numbers, not keys); alias tables for payees (`AMZN` vs `Amazon.ca`); a lock between concurrent syncs.

**Business Rules**: see below.

## Business Rules & Invariants
- P2d-1 (store): new tables `findings(id VARCHAR PRIMARY KEY, type VARCHAR NOT NULL, first_found_at TIMESTAMP NOT NULL, fixed_at TIMESTAMP)` and `finding_items(finding_id VARCHAR NOT NULL, transaction_id VARCHAR, split_id VARCHAR, payee_id VARCHAR, category_id VARCHAR)`; `finding_items` holds rows only for currently detected findings. `duckstore.FormatVersion` 3→4; read commands give the existing R2 line on a v3 store until the next sync (accepted). No `status` column and no fix text in the store. `transfers` gains nullable `other_account VARCHAR`: the other account's name Quicken recorded on a one-sided leg (NULL when paired), so the one-sided row's `other account:` label renders from the store (ruled at SCENARIO-11 planning).
- P2d-2 (identity): finding id = `<type>:<entity>` — split on the first `:`, no commas, shell-safe. IDs are a contract (users write them into config.toml); any later change to a normalisation rule that produces an id must be named in that feature's spec. Pair ids put the lower numeric source id first (numeric, not string, sort). `similar-categories` ids for income groups carry the constant entity prefix `income:` (`similar-categories:income:gift`); expense groups stay bare (`similar-categories:grocery`) — an id-rule change, named here (ruled at SCENARIO-27).
- P2d-3 (status, one core function, never stored): `fixed_at` set → `fixed` (even when ignored); else id in `findings.ignore` → `ignored`; else → `open`.
- P2d-4 (lifecycle): detected = present in this build's detection. A carried finding not detected gets `fixed_at` = this build's `store_info.built_at`; a fixed finding keeps its row forever, items gone. A fixed finding detected again: `fixed_at` cleared, `first_found_at` kept, not counted `new`. `new` = open findings with `first_found_at` = latest `built_at`; `newly_fixed` = findings with `fixed_at` = latest `built_at`. An ignore decision sticks to the id whatever its items become. Time is recorded as timestamps, not run ids.
- P2d-5 (detection): runs inside the build after validation, on the build file (like views), for `sync` and `sync --from`. A fault in detection is a build fault (existing build refusals, previous store untouched, exit 1). Open findings never make sync exit non-zero.
- P2d-6 (carry): findings carried from the previous store through the 2c carry seam (read-only, before the build file exists). Previous store has no `findings` table (format 3) → silent, history starts; no previous store → silent (CF1). Carry faults → warnings (see Surface & Copy), build proceeds; after a carry fault nothing is marked fixed and the sync line drops `(M new)` and the fixed clause.
- P2d-7 (types, one owner per rule, never re-derived):
  - `duplicate`: two transactions in the same account, equal non-zero `transactions.amount`, |d1 − d2| ≤ 3 days inclusive; excluded when both have status `reconciled`. Every account (closed, excluded-from-reports, linked-tracking included), transfer legs included, payee ignored. 3+ matches → one finding per pair. ID `duplicate:txn-A+txn-B`; items: 2 rows (`transaction_id`).
  - `one-sided-transfer`: exactly `transfers.to_split_id IS NULL` (same set as `sync --json` `store.transfers.one_sided`); every account. ID `one-sided-transfer:xfer-N`; items: 1 row (`transaction_id`, `split_id` of the from-split).
  - `unlinked-transfer`: transactions A, B in different accounts of the same currency, `A.amount = −B.amount ≠ 0`, |dA − dB| ≤ 3 days, neither has a split that is a transfer leg (in `transfers.from_split_id`/`to_split_id`) or has `transfer_account_id` set. Cross-currency pairs never flagged. One finding per pair; every account incl. closed. ID `unlinked-transfer:txn-A+txn-B`; items: 2 rows (`transaction_id`).
  - `uncategorized`: exactly the `v_cash_flow` rows with `category_id IS NULL` (inherits its exclusions); closed accounts and future dates included; they are the uncategorized splits `quarry cashflow` counts in its Income and Spent totals for a period (cashflow prints no separate uncategorized count). One finding per payee. ID `uncategorized:payee-N`, `uncategorized:no-payee`; items: one row per split (`transaction_id`, `split_id`).
  - `mixed-categories`: over categorized `v_cash_flow` rows, transactions with exactly one such row; a payee with ≥ 3 such transactions and ≥ 2 distinct categories whose category-change count (walk by date, then transaction source id) exceeds (distinct categories − 1). ID `mixed-categories:payee-N`; items: one row per category (`payee_id`, `category_id`, transaction count).
  - `payee-variants`: payees used by ≥ 1 transaction (any account); key = case-fold, cut at first `*` or `#`, non letter/digit → space, drop tokens containing a digit, collapse spaces, trim; empty key skipped; ≥ 2 payees with one key → one finding. Key details (ruled at SCENARIO-26 planning, part of the id contract): case-fold is Go `strings.ToLower`, no diacritic folding or Unicode normalisation (`Café` ≠ `Cafe`; a decomposed accent's combining mark is non-letter → space); letter/digit per Go `unicode.IsLetter`/`unicode.IsDigit`; non-letter/digit splits before the digit-token drop (`7-Eleven` → `eleven`, `7Eleven` → empty, skipped); two payees with identical names form a group. ID `payee-variants:<key tokens joined by ->`; items: one row per payee (`payee_id`, transaction count).
  - `similar-categories`: categories of kind `income` or `expense`, hidden included; `full_path` normalised per level: case-fold, non letter/digit → space, tokens, singularise tokens of ≥ 4 chars (`ies`→`y`; trailing `s` dropped unless `ss`), tokens joined `-`, levels joined `/`; ≥ 2 categories of the same kind with one key → one finding (income and expense never grouped). ID `similar-categories:<key>` for expense, `similar-categories:income:<key>` for income (no collision: keys never contain `:`); key details follow the payee key (ToLower, `unicode.IsLetter`/`IsDigit`, no normalisation) except digits are kept; the 4-character threshold counts runes; `:` splits levels; singularising is literal (`Taxes` → `taxe`); a path with no tokens is skipped; identical `full_path`s group; items: one row per category (`category_id`, split count over all accounts).
  - `unused-category`: a category of kind `income` or `expense` is unused when neither it nor any descendant is referenced by any imported split (any account, closed and excluded included), any investment transaction in the snapshot (the importer records category references from rows it counts but does not import), or scheduled transactions / budget items where the v9 reference maps them. Ruled at SCENARIO-28 planning (safety: fewer delete-advice reports wins): every category reference the v9 reference schema maps counts as use — `ZBUDGETLINEITEM.ZCATEGORYTAG`, `ZLOANSPLITENTRY.ZCATEGORY`, `ZACCOUNT.ZLOANINTERESTCATEGORY`, `ZQUICKFILLRULESPLITENTRY.ZCATEGORYTAG`, `ZPRODUCTSERVICE.ZCATEGORY`, `ZCUSTOMERCREDITLINEITEM.ZCATEGORY`, and entries of `SmartCashFlowTransaction` (and any other non-imported parent) in `ZCASHFLOWTRANSACTIONENTRY` — (`ZFITRANSACTION`'s category columns are bank attributes, not ZTAG references, and are not read); as do references to categories of any kind (a `system` child keeps its parent used); only `income`/`expense` categories are reported. A hidden category anywhere under an unused visible parent blocks that parent; a child under an unreported unused parent is not reported (parent rule read literally). Hidden categories and anything under a hidden parent are never reported. Only the top unused node is reported (unused, not hidden, parent used or absent); its unused subtree is its items. ID `unused-category:cat-N`; items: the category and each descendant (`category_id`). **Gate:** the v9 fixture must show a category referenced only by an investment transaction is not reported; if the importer cannot see those references, `unused-category` does not ship — stop and report, not descope.
- P2d-8 (named policy numbers, not config keys): `findings.match_days` = 3 (duplicate, unlinked-transfer); `findings.mixed_min` = 3.
- P2d-9 (ignore): `findings.ignore` in config.toml, array of strings; absent or `[]` = nothing ignored; quarry never writes config.toml; un-ignore = remove the id. Duplicate elements, `""`, unknown type prefixes and ids matching no finding are accepted (W1 warning in `findings` only), never refused.
- P2d-10 (config loading, replaces P2c-2): `sync` (and `--from`), `snapshots`, `snapshots prune`, `findings` load and validate the whole file (every C row refuses, exit 1; C3 warns). `status` loads best-effort for the ignored count only: never refuses, prints no C3 warnings; on any C refusal prints the status config warning, omits the ignored clause, `"ignored": null`. `accounts`, `spend`, `cashflow`, `sql` never load config. P2c-1 amended: known keys are exactly `snapshots.keep`, `quicken.path`, `findings.ignore`.
- P2d-11 (one-sided output folded): on a successful build, sync drops the `?` rows under `Transfers` and the one-sided stderr warning / `warnings[]` entry; the `Transfers` count line stays; `store.transfers.one_sided` stays in `sync --json`; the validation-failure block (`renderStoreFailure`) keeps its `?` rows.
- P2d-12 (CSV writer, one owner in `internal/cli`, shared by `sql --csv` and `findings --csv`): header = column names; a field is quoted when it contains `,`, `"`, CR or LF, or is the empty string; `"` doubled; NULL = unquoted empty field, `""` = empty string (custom quoter; `encoding/csv` cannot distinguish); values = `QueryValue.Text` except NULL; `\n \t \r` kept verbatim inside quotes; line ending `\n`; no BOM; zero rows → header only, exit 0; no formula-injection escaping; a record whose only field is NULL is written as `""` so no data line is blank (one-column results only; ruled at the gate).
- P2d-13 (`sql --csv` limit): under `--csv` the default is every row; an explicit `--limit` is honoured with the existing stderr truncation note.
- P2d-14 (real-file gate): the last scenario runs `quarry findings --status all` on the real Quicken file; for each heuristic type (`unlinked-transfer`, `mixed-categories`, `payee-variants`, `similar-categories`, `unused-category`) David reviews up to 20 findings; more than half false positives for a type → that type's rule is re-ruled (mid-feature ruling) before the gate round.

---

## Triage Brief
- IDs are `<prefix>-<Z_PK>` (acct-, cat-, payee-, txn-, split-, xfer-<from split source id>): deterministic from Quicken's primary keys; a Quicken payee merge plausibly re-keys (accepted, documented, not detected).
- Uncategorized = `splits.category_id IS NULL` (importer `splits.go:75`, `categories.go:32-88`); `v_cash_flow` (`duckstore/schema.go:138`) and `reportedAccount` (`schema.go:134`) own the reporting scope. One-sided = `transfers.to_split_id IS NULL` (`schema.go:67`, importer `transfers.go:23,130`); count persisted as `import_runs.transfers_one_sided`; legs in `store.Validation.Transfers.OneSided`.
- No computed similarity, duplicates, "unused" today; raw inputs: payees(id, source_id, name), categories(parent_id, full_path, kind, hidden), transactions(account_id, date, amount, payee_id, status), splits(category_id, amount, transfer_account_id).
- Carry seam: `duckstore/history.go:57` `readHistory` (read-only before `s.create`), `:96` `readRuns`, `historyFault` → `store.Replaced.HistoryFault` → CF2 via `snapshot/import.go` `importVerified` (`:108`). `build` takes `carried`. `checkFormat` refuses `format_version ≠ FormatVersion`.
- Sync output: `internal/cli/sync.go:84-152`; `renderStore` (`render.go:88`) → `writeTransfers` (`:101`) → `oneSidedRows` (`:307`) → `otherAccountLabel` (`:332`); `renderStoreFailure` (`:354`) also calls `writeTransfers` (`:377`). `Outcome.warnings` (`import.go:45`) → `oneSidedWarning` (`:67`, only caller `:51`), `historyRestartWarning` (`:61`); `Warnings()` → `sync.go:141`; `WarningsAbsolute()` (`auto_prune.go:84`) → `json.go:137`. V1 tail at `import.go:184` (`validationFailedRefusal`).
- `sql`: `cli/sql.go:63-95` RunE, `--limit` (`:97`, default 500, truncation note `:75-78,137`), Long `:26-55`; renderers `render_sql.go:18` `renderSQLTable` (escapes `\n \t \r`), `json_sql.go:12`; `store.QueryValue{Null, Text, Native}` (NULL text "NULL").
- `status`: `cli/status.go:22`, `render_status.go:19`, `json_status.go` (shape pinned). Read `Store` port `internal/report/store.go:11-22`; port guards `cmd/quarry/run.go:25-30`.
- Config: `internal/config` (2c; `parse.go` key walk, C1–C4/C2q/C2r/C2a/C2m/C2t/C2l, C3 key quoting); `internal/platform/atomicfile` exists (unused here).
- Tests pinning what changes: one-sided warning — `snapshot/import_test.go:60-118`, `auto_prune_faults_test.go:62,82,85`, `import_history_test.go:101`, `cmd/quarry/run_transfers_test.go:226-227`, `run_json_test.go:60-61,75`, `run_sync_prune_json_test.go:179`, `run_config_test.go:242,257,277,301,317,337` (use one-sided only as a convenient warning source — switch source, don't delete); success `?` rows — `cli/render_internal_test.go:248-251,266`, `run_transfers_test.go:222-224`; `oneSidedRows` stays (`render_internal_test.go:430-466`); `renderStoreFailure` stays (`:491-560`); V1 tail — `run_validation_test.go:174,291`, `run_json_test.go:182`, `run_transfers_test.go:271`, `snapshot/sync_and_import_test.go:503`; FormatVersion / table list — `duckstore/query_test.go:30`, `duckstore_test.go:179`, `open_test.go:180-186`, `run_store_info_test.go:73`, `run_status_json_test.go:100`.
- Becomes dead: `oneSidedWarning`. Live with changed contract: `writeTransfers` (success path prints the count line only; failure path count + `?` rows), `renderStore`. Stays live: `oneSidedRows`, `otherAccountLabel`, `historyRestartWarning`, `Warnings()`, `WarningsAbsolute()`, `renderSQLTable`, `newOneSidedDocuments`.
- Owed debts closed here: 2a STATE `sql.go` Long re-wrap; 2a STATE V1 tail copy.

## Product Verdict
**SHIP WITH CHANGES** (two passes, 2026-09-30); user decisions: A — every finding type in one 2d (overrules the 2d/2d' split); B — ignore decisions as `findings.ignore` in config.toml (overrules a quarry-written file; no ignore command); C — fold one-sided output into findings.
1. Heuristic types ship with the exact rules and id grammar in P2d-7; normalisation that produces an id is a contract.
2. `unused-category` ships only under its gate (P2d-7).
3. Status derived in one core function, never stored (P2d-3).
4. Detectors select from existing owners (`v_cash_flow`, `transfers`), never re-derive.
5. Detection faults are build faults; open findings never fail a sync.
6. One-sided warning and success-path `?` rows fold into findings; JSON field kept.
7. `--csv` means every row by default; NULL distinct from `""`; one shared writer.
8. `findings.ignore` with C6/C6e/C2t+ and W1; P2d-10 loading table.
9. Real-file gate as the last scenario (P2d-14).
10. Apply every *Changes to existing surfaces* item verbatim.

## Surface & Copy

`<config>` = `~/Library/Application Support/quarry/config.toml` (absolute in `--json`). All refusals use the existing `quarry: ` prefix; warnings `quarry: warning: `; `warnings[]` carries warnings without the prefix.

### `quarry findings`
- Use `findings`; Short `List what to clean up in Quicken`
- Long:
```
List the problems sync found in the Quicken data, as a worklist to fix in
Quicken; quarry never changes the data itself. Each finding names what it
is about and what to change. After you fix them in Quicken, run quarry
sync: findings it no longer finds are marked fixed.

quarry looks for:
  duplicate           two transactions in one account with the same amount,
                      dated within 3 days of each other, unless both are
                      reconciled
  one-sided-transfer  a transfer with no matching transaction in the other
                      account
  unlinked-transfer   two transactions in different accounts of the same
                      currency that look like one transfer (opposite
                      amounts, within 3 days) but are not linked as one
  uncategorized       splits with no category, one finding per payee;
                      quarry cashflow counts them as income or spending
  mixed-categories    a payee whose transactions go back and forth between
                      categories
  payee-variants      payees whose names differ only in case, punctuation,
                      spacing, or store and reference numbers
  similar-categories  categories whose names differ only in case,
                      punctuation, spacing or a plural
  unused-category     a category no transaction uses; check that no
                      scheduled transaction or budget uses it before you
                      delete it

To keep a finding off the list after checking it, add its id to
findings.ignore in ~/Library/Application Support/quarry/config.toml:

  [findings]
  ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]

It stays ignored across syncs; remove the id to list it again. quarry never
writes that file. Without --status, only open findings are listed; --csv
prints one row per item, for a spreadsheet.
```
- Examples:
```
  quarry findings
  quarry findings --type duplicate
  quarry findings --status all --csv > findings.csv
```
- Flags (local to `findings`):
  - `--status` default `open`; source ``"show only findings whose status is `status`: open, ignored, fixed or all"`` → renders `--status status   show only findings whose status is status: open, ignored, fixed or all (default "open")`.
  - `--type` default empty; source ``"show only findings of this `type`: duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, payee-variants, similar-categories or unused-category"`` → `--type type`.
  - `--csv`: `"print one row per transaction or split as CSV"`.
- Group order (text, JSON, CSV): `duplicate`, `one-sided-transfer`, `unlinked-transfer`, `uncategorized`, `mixed-categories`, `payee-variants`, `similar-categories`, `unused-category`; blank line between groups.
- Sort within groups: duplicate / unlinked-transfer — the pair's later date descending, then id; one-sided — date descending, then id; uncategorized — split count descending, then payee name case-insensitive, then id; mixed-categories — transactions descending, then payee name case-insensitive, then id; payee-variants — total transactions descending, then id; similar-categories — total splits descending, then id; unused-category — `full_path` case-insensitive, then id; fixed findings — `fixed_at` descending, then id.
- Layout: two-space gaps, no trailing spaces; amounts `formatMoney`, right-aligned per column; account labels `accountLabel`; payees `payeeLabel`; no CAD+USD sums; uncategorized rows carry no amount.
- Fix table (one table in the core library; JSON `fix` = sentence; text header = group form after `: `, shown only when the group lists ≥ 1 open finding):

| Type | JSON `fix` | Text header |
|---|---|---|
| duplicate | `Delete the extra one in Quicken, or ignore the pair if both are real` | `Possible duplicates (N): delete the extra one in Quicken, or ignore the pair if both are real` |
| one-sided-transfer | `Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file` | `One-sided transfers (N): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file` |
| unlinked-transfer | `Make the pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts` | `Unlinked transfers (N): make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts` |
| uncategorized | `Give this payee's splits a category in Quicken` | `Uncategorized (N payees, M splits): give each payee's splits a category in Quicken` |
| mixed-categories | `Pick one category for this payee's transactions in Quicken, or ignore it if the mix is intended` | `Payees in mixed categories (N): pick one category per payee in Quicken, or ignore a payee whose mix is intended` |
| payee-variants | `Rename these payees to one in Quicken and add a renaming rule, or ignore the group if they are different merchants` | `Payee variants (N groups): rename each group to one payee in Quicken and add a renaming rule` |
| similar-categories | `Merge these categories into one in Quicken, or ignore the group if they mean different things` | `Similar categories (N groups): merge each group into one category in Quicken` |
| unused-category | `No transaction uses it; check that no scheduled transaction or budget does, then delete it in Quicken, or ignore it to keep it` | `Unused categories (N): no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken` |

  Singulars: `1 group`, `1 payee`, `1 split`, `1 transaction`, `1 category`, `1 subcategory` (`humanize.Count`). Group header count `(N)` = findings listed.
- stdout (exit 0) example:
```
Possible duplicates (2): delete the extra one in Quicken, or ignore the pair if both are real
  duplicate:txn-4410+txn-4412
    2026-08-03  Chequing (CAD)  Hydro One           -142.17
    2026-08-05  Chequing (CAD)  HYDRO ONE NETWORKS  -142.17
  duplicate:txn-2001+txn-2003
    2019-01-10  Visa (CAD, closed)  Tim Hortons  -2.45
    2019-01-11  Visa (CAD, closed)  Tim Hortons  -2.45

One-sided transfers (1): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file
  one-sided-transfer:xfer-301  2024-02-01  Visa (CAD)  Payment  1,200.00  other account: Savings (not in this file)

Unlinked transfers (1): make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts
  unlinked-transfer:txn-5000+txn-5003
    2026-07-02  Chequing (CAD)  Visa payment       -500.00  Bills
    2026-07-03  Visa (CAD)      Payment thank you   500.00  Income:Other

Uncategorized (2 payees, 16 splits): give each payee's splits a category in Quicken
  uncategorized:payee-88  AMZN MKTP CA  12 splits  2019-03-02 to 2026-09-12
  uncategorized:no-payee  (no payee)     4 splits  2012-01-01 to 2020-05-05

Payees in mixed categories (1): pick one category per payee in Quicken, or ignore a payee whose mix is intended
  mixed-categories:payee-12  Costco  3 categories, 48 transactions
    Groceries  30 transactions
    Household  12 transactions
    Auto:Fuel   6 transactions

Payee variants (1 group): rename each group to one payee in Quicken and add a renaming rule
  payee-variants:tim-hortons  2 payees, 252 transactions
    TIM HORTONS #1234  212 transactions
    Tim Hortons         40 transactions

Similar categories (1 group): merge each group into one category in Quicken
  similar-categories:grocery  2 categories
    Groceries  812 splits
    Grocery     14 splits

Unused categories (2): no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken
  unused-category:cat-17  Auto:Parking
  unused-category:cat-40  Vacation (and 3 subcategories)

5 open findings; 4 ignored and 12 fixed not shown (--status all)
Ignore a finding by adding its id to findings.ignore in ~/Library/Application Support/quarry/config.toml; see quarry findings --help
```
- Row rules: one-sided row reuses the existing `?`-row columns without the `?`, incl. `otherAccountLabel`. Uncategorized: `N splits` and `first to last`, or just the date when equal. `unlinked-transfer` category cell: the split's `full_path`; `(uncategorized)` when NULL; `(split)` when the transaction has > 1 split. ` (and N subcategories)` only when N > 0.
- Footer: under `--status open`: `N open findings` (`1 open finding`), then `; ` and clauses `J ignored` and `K fixed` joined with ` and `, followed by ` not shown (--status all)`; zero clauses omitted; both zero → no `;` part. Under `--status all`: `21 findings: 5 open, 4 ignored, 12 fixed` (zero clauses omitted). Under `--status ignored|fixed`: `N ignored findings` / `N fixed findings`. Footer counts follow `--type`. Hint line (text mode only, when ≥ 1 open finding is listed — never under `--status ignored|fixed`, never with an empty line — and `findings.ignore` is unset or empty): `Ignore a finding by adding its id to findings.ignore in ~/Library/Application Support/quarry/config.toml; see quarry findings --help`.
- Status markers: under `--status all` an ignored finding's id line ends `  ignored`; a fixed finding is one line `  <id>  fixed 2026-10-01` (local date of `fixed_at`), no item lines; open carries no marker.
- Ruled mid-feature (SCENARIO-18): the `  ignored` / `  fixed <date>` marker is always at the very end of the line, two spaces after the last column, on every row shape (multi-line types: on the id line); no marker under `--status ignored`. Within a group: all open, then all ignored (each in the type's sort), then fixed (`fixed_at` desc, id). Group header with no open finding listed: heading and `(N)` only, no `: fix` clause (e.g. `Possible duplicates (2)`); uncategorized drops `, M splits` when M = 0 (`Uncategorized (1 payee)`); N counts listed findings, M listed items.

| Input | stdout | stderr | Exit |
|---|---|---|---|
| no findings at all (default) | `No open findings` | nothing | 0 |
| none open, some ignored or fixed | `No open findings; 4 ignored and 12 fixed not shown (--status all)` | nothing | 0 |
| `--type duplicate`, none | `No open findings of type duplicate` (under `--status all`: `No findings of type duplicate`) | nothing | 0 |
| `--status fixed`, none | `No fixed findings` | nothing | 0 |
| `--status all`, nothing at all | `No findings` | nothing | 0 |
| `--status ignored`, none | `No ignored findings` | nothing | 0 |
| `--status ignored\|fixed --type duplicate`, none | `No ignored findings of type duplicate` / `No fixed findings of type duplicate` (no `not shown` clause under `--status ignored\|fixed\|all`) | nothing | 0 |
| default view `--type duplicate`, none open but some ignored/fixed of that type | `No open findings of type duplicate; 1 ignored and 2 fixed not shown (--status all)` (counts follow `--type`; zero clauses omitted) | nothing | 0 |
| finding in a closed account | listed; label `(CAD, closed)` | | 0 |
| USD account | label `(USD)`; amounts native | | 0 |
| no store / R2 (v3 store before the first 2d sync) / R3 | empty | existing store refusal lines, command `findings` | 1 |
| config C1, C2, C2q, C2r, C2t, C4, C6, C6e | empty | the C line | 1 |
| config C3 / W1 | listed | the warning line(s) | 0 |
| positional argument | empty | `quarry: findings takes no arguments; to ignore a finding add its id to findings.ignore in ~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.` | 2 |
| bad `--status` | empty | `quarry: --status must be open, ignored, fixed or all` | 2 |
| bad `--type` | empty | `quarry: --type must be duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, payee-variants, similar-categories or unused-category` | 2 |
| `--csv --json` | empty | `quarry: --csv and --json cannot be used together; choose one output format` | 2 |
| interrupted | empty | `quarry: findings interrupted` | 1 |
| stale store | listed as of that store; no staleness line | nothing | 0 |

- `--json` (one document type; the Phase 3 `data_quality` shape):
```json
{
  "status": "open",
  "type": null,
  "counts": {"open": 5, "ignored": 4, "fixed": 12, "new": 2, "newly_fixed": 1},
  "findings": [
    {
      "id": "duplicate:txn-4410+txn-4412",
      "type": "duplicate",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Delete the extra one in Quicken, or ignore the pair if both are real",
      "items": [
        {"transaction_id": "txn-4410", "split_id": null, "payee_id": null, "category_id": null,
         "date": "2026-08-03", "account_id": "acct-3", "account": "Chequing", "currency": "CAD",
         "payee": "Hydro One", "category": null, "amount": "-142.17",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null}
      ]
    }
  ],
  "warnings": []
}
```
  `type` = `--type` value or null; `counts` follows `--type`, not `--status`; item keys always present, null where not applicable; amounts are strings; `transactions`/`splits` are counts for payee and category items; a fixed finding has `"items": []`.
- `--csv` header: `finding_id,type,status,date,account,currency,payee,category,amount,other_account,transactions,splits,transaction_id,split_id,payee_id,category_id,fix`; one row per item; a fixed finding gets one row with item fields empty (NULL); same filters and order as text; P2d-12 writer.

### Sync
- Detection runs in the build after validation (P2d-5). New line after `Transfers`, before `Pruned` (`%-10s` label): clauses joined with `, `: `N open` or `none open`; ` (M new)` only when M > 0 and findings history was carried; `K fixed since the last sync` (K > 0); `J ignored` (J > 0); tail `; run quarry findings to list them` when N > 0. Examples: `Findings  12 open (3 new), 2 fixed since the last sync, 4 ignored; run quarry findings to list them`; `Findings  none open, 2 fixed since the last sync`; `Findings  none open`; first 2d sync / after CF1 or a carry fault: `Findings  12 open; run quarry findings to list them`.
- `--json`: `store.findings` = `{"open","ignored","fixed","new","newly_fixed"}`; key always present; `null` when `built` is false; never null otherwise (a bad config refuses sync first).
- Carry outcomes (warning, exit 0, build proceeds; each in `warnings[]` without prefix):

| Case | Line |
|---|---|
| previous store unreadable at open (store-level fault) | replaces CF2 for this case: `quarry: warning: cannot carry import history and findings forward from the previous store (<reason>); both start again with this sync` |
| only `import_runs` faulty | the existing CF2 line, unchanged |
| `import_runs` and `findings` both faulty (store opens) | two lines, each with its own reason: the existing CF2 line, then the findings-only line; stderr order manifest, CF2, findings, prune; two `warnings[]` entries in that order (ruled mid-feature, SCENARIO-08) |
| only `findings` faulty | `quarry: warning: cannot carry findings forward from the previous store (<reason>); findings history starts again with this sync` |
| `findings` reasons | ids not unique: `its findings table repeats an id`; a required column (`id`, `type`, `first_found_at`) missing or NULL, or any other row fault: `its findings table is incomplete` |
| previous store has no `findings` table (format 3) | silent; history starts |
| no previous store | silent (CF1) |

- Edge rows: `sync --from <older snapshot>` marks findings fixed that are not in it and reopens old ones (reopened keep `first_found_at`, not `new`; `newly_fixed` may spike, accepted). A Quicken payee merge re-keys a payee: `uncategorized:payee-A` fixed, `uncategorized:payee-B` new (accepted, documented).

### `status`
- New line after `Transfers`: `Findings  12 open, 4 ignored; run quarry findings to list them`; `Findings  none open`; `Findings  none open, 4 ignored`. No new/fixed clauses.
- `status --json` gains `"findings": {"open","ignored","fixed","new","newly_fixed"}` after `transfers` (additive; existing fields byte-identical); `ignored` null on a config refusal.
- Best-effort config (P2d-10): on any C refusal, stderr `quarry: warning: cannot tell which findings you ignored: <the C line without its "quarry: " prefix and without "; fix the file and run the command again">; findings you ignored are counted as open` — e.g. `quarry: warning: cannot tell which findings you ignored: ~/Library/Application Support/quarry/config.toml: snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open`; ignored clause omitted; exit 0.
- Long first paragraph ends: `…the dates its transactions cover, the checks sync ran when it built the store, and how many findings are open.`
- Long paragraph 2 replaced (ruled mid-feature, SCENARIO-19):
```
status reads quarry's store, and the config file for the findings you ignored;
it never looks at Quicken. Run quarry sync to bring the store up to date.
```
- `status --json` `warnings[]` (existing, today always `[]`) carries the config warning unprefixed, one entry, on any C refusal; stderr still prints it; C3 and W1 never reach it. `<config>` in that entry uses the same form as the other config warnings in `--json` (abbreviated today; see STATE open debt on the 2a absolute-path rule).

### Config: `findings.ignore`
| # | Condition | stderr | Exit |
|---|---|---|---|
| C6 | `findings.ignore` is not an array (`"x"`, `12`, `true`) | `quarry: <config>: findings.ignore must be a list of finding ids in quotes, such as ["duplicate:txn-4410+txn-4412"], got "duplicate:txn-4410+txn-4412"; fix the file and run the command again` | 1 |
| C6 (table header) | `[findings.ignore]` / `[[findings.ignore]]` | the same line ending `got a table` / `got a list of tables` (C2a/C2l) | 1 |
| C6 (multi-line) | multi-line raw value | collapsed per C2m | 1 |
| C6e | an element is not a string | `quarry: <config>: findings.ignore must hold only finding ids in quotes, got 12 as item 3; fix the file and run the command again` (items counted from 1; raw element collapsed per C2m) | 1 |
| C2t+ | `findings = 3` / `findings = "x"` | `quarry: <config>: findings must be a table, such as findings.ignore = ["duplicate:txn-4410+txn-4412"], got 3; fix the file and run the command again` (C2l kinds apply) | 1 |
| C3 | unknown key under `[findings]` (e.g. `findings.ignored`) | the existing unknown-key warning | 0 |
| W1 (`findings` only) | an element matches no row in `findings` (any status) | one line per element, file order, id as a TOML basic string: `quarry: warning: <config>: findings.ignore lists "uncategorized:payee-999", which is not a finding in quarry's store; quarry skips it` | 0 |

Duplicates, `""` and unknown type prefixes accepted silently (W1 covers unmatched). `sync` and `status` are silent about W1.

### `quarry sql --csv`
- `--csv` help: `"print the rows as CSV, with a header line"`.
- `--limit` help → ``"print at most `n` rows (500 unless set, every row with --csv; 0 prints every row)"``.
- New example line: `  quarry sql --csv "SELECT * FROM transactions" > transactions.csv`
- Writer per P2d-12; limit per P2d-13; `--csv --json` → the `findings` usage line, exit 2; an unprintable value takes the existing refusal.

### Changes to existing surfaces
1. sync Long — insert after the "if a check fails, the previous store is left unchanged." paragraph:
```
sync then looks for things to clean up in Quicken, such as uncategorized
splits, one-sided transfers and possible duplicates; run quarry findings to
list them. Findings never fail a sync.
```
2. sql Long — replace from the paragraph starting "Amounts are DECIMAL" to the end (closes the 2a re-wrap debt; lines ≤ 80 columns):
```
Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. For spending and income, query v_spending and
v_cash_flow: they already leave out transfers between your own accounts,
Quicken's system categories, transactions excluded from reports and
accounts Quicken leaves out of reports, so their totals match quarry spend
and quarry cashflow. A transfer leg is any split named in
transfers.from_split_id or transfers.to_split_id.

findings holds what sync found to clean up in Quicken, and finding_items
the transactions, splits, payees or categories each one is about;
fixed_at is set once a finding is no longer found. Which findings you
ignored is set in the config file, not the store: quarry findings shows
each one's status.

List the tables and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set, every row with --csv);
when there are more, quarry says so on stderr. --limit 0 prints every row.
With --csv, an empty field is NULL and "" is an empty string, except in a
one-column result, where NULL is also written as "" so no row is blank.
```
3. V1 tail (`internal/snapshot/import.go` `validationFailedRefusal`): `fix the account in Quicken and run quarry sync, …` → `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry` (closes the 2a STATE debt; update pinned tests).
4. CF2 `historyRestartWarning`: the store-level case gets the combined line above.
5. `oneSidedWarning` removed; success-path `?` rows removed from `renderStore` (P2d-11); `oneSidedRows` and `otherAccountLabel` stay for `renderStoreFailure`.
6. status Long per the `status` section.
7. Root Long, `accounts`, `spend`, `cashflow`, `snapshots` Longs: unchanged, still true.
8. PRD `docs/initial-prd.md`: :126 `findings` row → `` `findings`, `finding_items` | What each sync found to clean up: id, type, when first found, when fixed; `finding_items` names the transactions, splits, payees or categories | Status (open, fixed, ignored) and the suggested fix come from `quarry findings`; ignore decisions live in the config file ``; :135 → "Real fixes happen in Quicken. `quarry` stores only decisions about findings: the ids the user lists under `findings.ignore` in `config.toml`, which quarry reads and never writes, so no second, divergent copy of the truth builds up and rebuilding the store loses no decision."; :172 `findings` row → "The cleanup worklist to apply in Quicken; `--csv` to export; ignore a finding by listing its id under `findings.ignore` in the config file"; :228 duplicates row → add ", unless both are reconciled"; :229 → "Transfer booked as income or expense (`unlinked-transfer`) …"; :232 → "Findings have a status: open, fixed (gone on re-import), or ignored (id listed in `findings.ignore`; remove it to list the finding again)."; milestones: 2d covers every finding type; add the `unused-category` safety rule (importer investment-reference condition, hidden excluded, check-first fix copy).
9. 2c spec P2c-1/P2c-2 amended per P2d-10 (note in 2c STATE at SHIP).

---

## Scenarios (Gherkin)

### Store, sync and lifecycle

```gherkin
Scenario: SCENARIO-01 — sync records findings and prints a Findings line
  Given a Quicken file with two uncategorized splits from one payee and a one-sided transfer
  When I run quarry sync
  Then the store's findings table holds uncategorized:payee-N and one-sided-transfer:xfer-N, stdout has "Findings  2 open; run quarry findings to list them" after Transfers, and the exit code is 0
```

```gherkin
Scenario: SCENARIO-02 — a transfer with no matching leg is a one-sided-transfer finding
  Given a transfer whose other account has no matching transaction
  When I run quarry sync
  Then the store holds one-sided-transfer:xfer-N with the from-split as its item
```

```gherkin
Scenario: SCENARIO-03 — uncategorized findings count what cashflow counts
  Given uncategorized splits in reported and not-reported accounts
  When I run quarry sync
  Then each uncategorized:<payee> finding's items are exactly that payee's v_cash_flow rows with no category, and, where only uncategorized splits flow, their sums by sign equal quarry cashflow's Income and Spent
```

```gherkin
Scenario: SCENARIO-04 — a successful sync lists one-sided transfers only as findings
  Given a Quicken file with a one-sided transfer that validates
  When I run quarry sync
  Then stdout's Transfers block has the count line and no "?" rows, stderr has no one-sided warning, and sync --json still carries store.transfers.one_sided
```

```gherkin
Scenario: SCENARIO-05 — a validation failure says to fix them in Quicken
  Given a Quicken file whose splits do not reconcile
  When I run quarry sync
  Then stderr ends "fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry" and the exit code is 1
```

```gherkin
Scenario: SCENARIO-06 — a finding no longer found is marked fixed on the next sync
  Given a store whose findings include uncategorized:payee-N, and the payee's splits are now categorized in Quicken
  When I run quarry sync
  Then that finding has fixed_at set to the build time, its items are gone, and stdout's Findings line has "1 fixed since the last sync"
```

```gherkin
Scenario: SCENARIO-07 — a fixed finding that comes back reopens and is not new
  Given a store where uncategorized:payee-N was fixed, and the payee has an uncategorized split again
  When I run quarry sync
  Then the finding is open with its original first_found_at and the Findings line has no "(1 new)"
```

```gherkin
Scenario: SCENARIO-08 — findings history that cannot be carried forward restarts with a warning
  Given a previous store whose findings table repeats an id
  When I run quarry sync --from <id>
  Then stderr is "quarry: warning: cannot carry findings forward from the previous store (its findings table repeats an id); findings history starts again with this sync" and the exit code is 0
```

### Duplicates

```gherkin
Scenario: SCENARIO-09 — two same-amount transactions within 3 days are a possible duplicate
  Given two transactions in one account of -142.17 dated 2026-08-03 and 2026-08-05
  When I run quarry sync
  Then the store holds duplicate:txn-A+txn-B with both transactions as items
```

```gherkin
Scenario: SCENARIO-10 — two reconciled look-alikes are not a duplicate
  Given two reconciled transactions in one account of -142.17 dated a day apart
  When I run quarry sync
  Then no duplicate finding names them
```

### `quarry findings`

```gherkin
Scenario: SCENARIO-11 — findings lists open findings by type with their fix
  Given a store with an open duplicate, one-sided transfer and uncategorized finding
  When I run quarry findings
  Then stdout has the three groups in order with their headers and rows, the footer "3 open findings", and the ignore hint line
```

```gherkin
Scenario: SCENARIO-12 — findings with none open says so
  Given a store with no findings
  When I run quarry findings
  Then stdout is "No open findings" and the exit code is 0
```

```gherkin
Scenario: SCENARIO-13 — findings refuses a store built before findings existed
  Given a store of format 3
  When I run quarry findings
  Then stderr is the existing R2 line with its sync --from fix and the exit code is 1
```

```gherkin
Scenario: SCENARIO-14 — findings --json returns the worklist as a document
  Given a store with one open duplicate finding
  When I run quarry findings --json
  Then stdout is the ruled document with status, type, counts, findings (id, type, status, first_found_at, fixed_at, fix, items) and warnings
```

### Ignore via config

```gherkin
Scenario: SCENARIO-15 — a malformed findings.ignore is refused before Quicken is touched
  Given config.toml sets findings.ignore = "duplicate:txn-1+txn-2"
  When I run quarry sync
  Then stderr is the C6 line, no snapshot is taken, and the exit code is 1
```

```gherkin
Scenario: SCENARIO-16 — an id listed in findings.ignore is ignored
  Given a store with open findings duplicate:txn-1+txn-2 and uncategorized:payee-8, and config.toml lists "duplicate:txn-1+txn-2" in findings.ignore
  When I run quarry findings
  Then only uncategorized:payee-8 is listed and the footer is "1 open finding; 1 ignored not shown (--status all)"
```

```gherkin
Scenario: SCENARIO-17 — an ignored id that is not a finding warns
  Given config.toml lists "uncategorized:payee-999" in findings.ignore and no such finding exists
  When I run quarry findings
  Then stderr is the W1 line for "uncategorized:payee-999" and the exit code is 0
```

```gherkin
Scenario: SCENARIO-18 — findings filters by status and type
  Given a store with open, ignored and fixed findings of several types
  When I run quarry findings --status all --type duplicate
  Then only duplicate findings are listed, ignored ones marked "ignored", fixed ones as one "fixed <date>" line, and the footer counts duplicates only
```

### `status`

```gherkin
Scenario: SCENARIO-19 — status shows the open findings count
  Given a store with 3 open findings and 1 ignored
  When I run quarry status
  Then stdout has "Findings  3 open, 1 ignored; run quarry findings to list them" and status --json carries "findings" after "transfers"
```

```gherkin
Scenario: SCENARIO-20 — status warns on a bad config and still reports
  Given a store with open findings and config.toml setting snapshots.keep = 0
  When I run quarry status
  Then stderr is the "cannot tell which findings you ignored" warning, the Findings line has no ignored clause, and the exit code is 0
```

### CSV

```gherkin
Scenario: SCENARIO-21 — sql --csv prints every row with a header
  Given a store with 600 transactions, a payee containing a comma and a quote, and a NULL memo beside an empty one
  When I run quarry sql --csv "SELECT id, payee, memo FROM ..."
  Then stdout is the header and all 600 rows, the comma and quote field quoted with the quote doubled, NULL as an empty field and "" as an empty string, and stderr is empty
```

```gherkin
Scenario: SCENARIO-22 — findings --csv prints one row per item
  Given a store with one open duplicate and one fixed finding
  When I run quarry findings --status all --csv
  Then stdout is the ruled header, two rows for the duplicate's transactions and one row for the fixed finding with empty item fields
```

```gherkin
Scenario Outline: SCENARIO-23 — findings rejects usage it cannot use
  Given any store
  When I run quarry findings <args>
  Then stderr is <line> and the exit code is 2
  Examples:
    | args            | line                                                                                              |
    | extra           | the positional-argument line                                                                      |
    | --status maybe  | "quarry: --status must be open, ignored, fixed or all"                                            |
    | --type bogus    | the --type line naming all eight types                                                            |
    | --csv --json    | "quarry: --csv and --json cannot be used together; choose one output format"                      |
```

### Heuristic finding types

```gherkin
Scenario: SCENARIO-24 — opposite amounts in two accounts not linked as a transfer are an unlinked transfer
  Given -500.00 in Chequing (CAD) on 2026-07-02 and +500.00 in Visa (CAD) on 2026-07-03, neither a transfer, and the same pair across a CAD and a USD account
  When I run quarry sync
  Then the store holds unlinked-transfer:txn-A+txn-B for the CAD pair only
```

```gherkin
Scenario: SCENARIO-25 — a payee whose category goes back and forth is in mixed categories
  Given payee Costco with transactions Groceries, Household, Groceries, Auto:Fuel, and payee Shell with Auto for years then Auto:Fuel after
  When I run quarry sync
  Then the store holds mixed-categories:payee-<Costco> and no finding for Shell
```

```gherkin
Scenario: SCENARIO-26 — payees that differ only in case, punctuation or numbers are variants
  Given payees "TIM HORTONS #1234" and "Tim Hortons", each used by a transaction
  When I run quarry sync
  Then the store holds payee-variants:tim-hortons naming both payees
```

```gherkin
Scenario: SCENARIO-27 — categories that differ only in case, punctuation or a plural are similar
  Given expense categories "Groceries" and "Grocery", and an income category "Grocery"
  When I run quarry sync
  Then the store holds similar-categories:grocery naming the two expense categories only
```

```gherkin
Scenario: SCENARIO-28 — an unused category is reported only when nothing imported or counted uses it
  Given an expense category used by no split, one used only by an investment transaction, one hidden and unused, and an unused parent with an unused child
  When I run quarry sync
  Then the store holds unused-category for the first category and for the parent (with the child as an item) only
```

---

## Sizing
Ruled: the finding core (type list and order, id grammar, P2d-3 status function, fix table, counts) is a leaf package `internal/finding` importing only the standard library; `snapshot`, `report`, `duckstore` and the new `findings` feature package import it. Text rows, JSON item fields and CSV cells of a type ship with the scenario that detects it when that scenario comes after 11/14/22; types detected earlier (duplicate, one-sided, uncategorized) get their rows from 11, 14 and 22.

| Scenario (was) | Verdict — numbers |
| --- | --- |
| 01 (01) | OWNS A RUN — 4 batches (tables + FormatVersion 3→4 + pin fallout; detection hook + one-sided + uncategorized detectors; build result counts; sync `Findings` line `N open` + sync Long insert, change 1), `snapshot` + duckstore + cli; PRD change 8 in its Sweep except the unused-category line (28) |
| 02 (21) | FOLD into 01 — the one-sided detector's item row; acceptance test folded |
| 03 (23) | FOLD into 01 — scope test of the uncategorized detector against `quarry cashflow`'s count |
| 04 (04) | OWNS A RUN — 3 batches (drop `oneSidedWarning` + repoint ~10 pinned tests to another warning source; drop success `?` rows; sync `--json` `store.findings`), `snapshot` + cli |
| 05 (06) | FOLD into 04 — V1 tail copy (change 3), one string + pinned tests in the same files |
| 06 (02) | OWNS A RUN — 3 batches (carry findings read in `history.go`; merge: fixed_at / reopen / new / newly_fixed; `(M new)` and `K fixed since the last sync` clauses), duckstore + `snapshot` + cli |
| 07 (03) | FOLD into 06 — reopen is one branch of 06's merge |
| 08 (05) | OWNS A RUN — 3 batches (findings fault reasons; `store.Replaced` findings fault + store-level combined line replacing CF2, change 4; drop `(M new)`/fixed clause after a fault), duckstore + `snapshot` |
| 09 (19) | LIGHT — 2 steps, duckstore (pair detector, numeric pair-id order, 3/4-day bound tests) |
| 10 (20) | FOLD into 09 — reconciled exclusion is one predicate |
| 11 (07) | OWNS A RUN — 4 batches (store read port + duckstore adapter + contract test; `findings` package list/sort/counts; command, Long, Examples, registers AND validates `--status`/`--type` + positional refusal, each pinned here; text groups/rows for duplicate, one-sided, uncategorized + footer + hint), new `findings` package + cli + cmd wiring |
| 12 (11) | FOLD into 11 — empty-state line |
| 13 (13) | FOLD into 11 — R2 falls out of the existing format check (`open_test.go:180` already builds a FormatVersion-1 store) |
| 14 (09) | LIGHT — 2 steps (`--json` document; item fields for the three existing types), cli |
| 15 (16) | OWNS A RUN — 3 batches (`findings.ignore` array + C6/C6e; C6 table/multi-line, C2t+; known keys + C3 under `[findings]`), `internal/config` |
| 16 (14) | OWNS A RUN — 3 batches (status `ignored` branch + findings footer clause; W1; sync `J ignored` clause + sync `--json` `ignored` via a snapshot option), `findings` + cli, `snapshot` option only |
| 17 (15) | FOLD into 16 — W1 is the unmatched arm of 16's match |
| 18 (08) | OWNS A RUN — 3 batches (filters; `ignored` marker + `fixed <date>` lines + fixed sort; footer and empty lines for `--status all/ignored/fixed` and `--type`), `findings` + cli |
| 19 (17) | OWNS A RUN — 4 batches (report port read + adapter; status line + `status --json` `findings`; best-effort config load + warning; status Long, change 6), `report` + cli + `cmd/quarry` port guards |
| 20 (18) | FOLD into 19 — best-effort warning is one branch of 19's load |
| 21 (28) | OWNS A RUN — 3 batches (P2d-12 writer + quoting matrix; `--csv` + `--limit` default + `--csv --json` usage line; sql Long, change 2), cli |
| 22 (10) | LIGHT — 3 steps (`--csv` rows incl. fixed row; `--csv --json`; outline test), cli |
| 23 (12) | FOLD into 22 — outline's last row (`--csv --json`) lands in 22; its other rows are 11's code |
| 24 (22) | OWNS A RUN — 3 batches (detector, currency and transfer-leg exclusions; text rows + category cell; JSON/CSV cells), duckstore + `findings` + cli |
| 25 (24) | OWNS A RUN — 3 batches (change-count walk; payee/category rows; JSON/CSV counts), duckstore + `findings` + cli |
| 26 (25) | OWNS A RUN — 3 batches (normalisation key, an id contract; detector; rows), duckstore + `findings` + cli |
| 27 (26) | OWNS A RUN — 3 batches (per-level key with singularising, kind split; detector; rows), duckstore + `findings` + cli |
| 28 (27) | OWNS A RUN — 4 batches (`importer` records category ids of counted, not-imported investment entries — today `mapSplits` drops them — as a `store.Rows` field, no new table; detector with hidden-subtree and top-unused-node rules; rows; PRD unused-category line), `importer` + duckstore + `findings` + cli. Gate (P2d-7) proven by the acceptance test's investment-only category, built with the v9 fixture's `EntInvestmentTransaction`; scheduled/budget references are not mapped by the v9 reference, so the check-first fix copy covers them |
| 29 (29) | Moved to `## Reference check` — manual review on the real file, no Go acceptance test, no architect or developer run |

## BDD Acceptance Progress
- [x] SCENARIO-01: sync records findings and prints a Findings line — `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_records_findings_and_prints_the_findings_line`
- [x] SCENARIO-02: a transfer with no matching leg is a one-sided-transfer finding — delivered by SCENARIO-01 — `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_records_a_one_sided_transfer_with_its_from_split_as_the_item`
- [x] SCENARIO-03: uncategorized findings count what cashflow counts — delivered by SCENARIO-01 — `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_uncategorized_findings_hold_what_cashflow_counts`
- [x] SCENARIO-04: a successful sync lists one-sided transfers only as findings — `cmd/quarry/run_transfers_test.go` `Test_run_lists_one_sided_transfers_only_as_findings_on_a_successful_sync`
- [x] SCENARIO-05: a validation failure says to fix them in Quicken — delivered by SCENARIO-04 — `cmd/quarry/run_validation_test.go` `Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block`
- [x] SCENARIO-06: a finding no longer found is marked fixed on the next sync — `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_marks_a_finding_no_longer_found_fixed_at_the_build_time`
- [x] SCENARIO-07: a fixed finding that comes back reopens and is not new — delivered by SCENARIO-06 — `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_reopens_a_fixed_finding_with_its_first_found_at_and_not_new`
- [x] SCENARIO-08: findings history that cannot be carried forward restarts with a warning — `cmd/quarry/run_findings_carry_test.go` `Test_run_sync_from_warns_and_restarts_findings_history_when_the_previous_findings_table_repeats_an_id`
- [x] SCENARIO-09: two same-amount transactions within 3 days are a possible duplicate — `cmd/quarry/run_sync_duplicates_test.go` `Test_run_sync_records_two_same_amount_transactions_within_three_days_as_a_duplicate`
- [x] SCENARIO-10: two reconciled look-alikes are not a duplicate — delivered by SCENARIO-09 — `cmd/quarry/run_sync_duplicates_test.go` `Test_run_sync_does_not_flag_two_reconciled_look_alikes_as_a_duplicate`
- [x] SCENARIO-11: findings lists open findings by type with their fix — `cmd/quarry/run_findings_test.go` `Test_run_findings_lists_open_findings_by_type_with_their_fix`
- [x] SCENARIO-12: findings with none open says so — delivered by SCENARIO-11 — `cmd/quarry/run_findings_test.go` `Test_run_findings_says_no_open_findings_when_the_store_has_none`
- [x] SCENARIO-13: findings refuses a store built before findings existed — delivered by SCENARIO-11 — `cmd/quarry/run_findings_test.go` `Test_run_findings_refuses_a_store_built_before_findings_existed`
- [x] SCENARIO-14: findings --json returns the worklist as a document — `cmd/quarry/run_findings_json_test.go` `Test_run_findings_json_prints_the_ruled_document_for_one_open_duplicate`
- [x] SCENARIO-15: a malformed findings.ignore is refused before Quicken is touched — `cmd/quarry/run_findings_ignore_config_test.go` `Test_run_sync_refuses_a_findings_ignore_that_is_not_a_list_before_taking_a_snapshot`
- [x] SCENARIO-16: an id listed in findings.ignore is ignored — `cmd/quarry/run_findings_ignore_test.go` `Test_run_findings_leaves_an_ignored_id_off_the_list_and_counts_it_in_the_footer`
- [x] SCENARIO-17: an ignored id that is not a finding warns — delivered by SCENARIO-16 — `cmd/quarry/run_findings_ignore_test.go` `Test_run_findings_warns_about_an_ignored_id_that_is_not_a_finding`
- [x] SCENARIO-18: findings filters by status and type — `cmd/quarry/run_findings_filters_test.go` `Test_run_findings_status_all_type_duplicate_marks_ignored_shows_fixed_as_a_date_line_and_counts_duplicates_only`
- [x] SCENARIO-19: status shows the open findings count — `cmd/quarry/run_status_findings_test.go` `Test_run_status_shows_the_findings_line_with_open_and_ignored_counts`
- [x] SCENARIO-20: status warns on a bad config and still reports — delivered by SCENARIO-19 — `cmd/quarry/run_status_findings_test.go` `Test_run_status_warns_on_a_bad_config_and_still_reports`
- [x] SCENARIO-21: sql --csv prints every row with a header — `internal/cli/sql_csv_test.go` `Test_sql_csv_prints_every_row_with_a_header`
- [x] SCENARIO-22: findings --csv prints one row per item — `cmd/quarry/run_findings_csv_test.go` `Test_run_findings_status_all_csv_prints_a_row_per_duplicate_item_and_one_row_for_the_fixed_finding`
- [x] SCENARIO-23: findings rejects usage it cannot use — delivered by SCENARIO-22 — `cmd/quarry/run_findings_usage_test.go` `Test_run_findings_rejects_usage_it_cannot_use`
- [x] SCENARIO-24: opposite amounts in two accounts not linked as a transfer are an unlinked transfer — `cmd/quarry/run_sync_unlinked_test.go` `Test_run_sync_records_opposite_amounts_in_two_cad_accounts_as_an_unlinked_transfer_and_not_the_cad_usd_pair`
- [x] SCENARIO-25: a payee whose category goes back and forth is in mixed categories — `cmd/quarry/run_sync_mixed_test.go` `Test_run_sync_records_costco_as_mixed_categories_and_not_shell`
- [x] SCENARIO-26: payees that differ only in case, punctuation or numbers are variants — `cmd/quarry/run_sync_payee_variants_test.go` `Test_run_sync_records_tim_hortons_variants_and_not_unrelated_payees`
- [x] SCENARIO-27: categories that differ only in case, punctuation or a plural are similar — `cmd/quarry/run_sync_similar_categories_test.go` `Test_run_sync_records_similar_expense_categories_and_not_the_income_one`
- [x] SCENARIO-28: an unused category is reported only when nothing imported or counted uses it — `cmd/quarry/run_sync_unused_category_test.go` `Test_run_sync_records_unused_categories_but_not_one_an_investment_or_budget_uses`

## Reference check
Last step before the gate round (step 7), per the reference-check rule; not a BDD scenario because it has no Go acceptance test. Outcome recorded in `REFERENCE-CHECK.md` beside this file.

- [x] Findings on the real Quicken file hold up to review (see `REFERENCE-CHECK.md`, 2026-10-01): David runs `quarry sync` then `quarry findings --status all` on his Quicken file and reviews up to 20 findings per heuristic type (`unlinked-transfer`, `mixed-categories`, `payee-variants`, `similar-categories`, `unused-category`). A type with more than half false positives is re-ruled (mid-feature product-vision ruling) and the fix appended as a new scenario before the gate round.
