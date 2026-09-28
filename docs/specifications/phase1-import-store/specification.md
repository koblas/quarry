# Specification: Phase 1 — Import + store

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` rebuilds quarry's own DuckDB store, `~/Library/Application Support/quarry/quarry.duckdb`, from the snapshot it just took — banking, credit, splits, transfers, categories, payees and tags — and swaps it in only when the store reconciles with Quicken. `quarry sync --from <snapshot>` rebuilds from an earlier snapshot without reading Quicken. Every later phase trusts this store, so a store that does not reconcile never replaces the previous one.

**Out of Scope** (each with the phase it lands by):
- Reconciliation at every past statement date; row-count / date-range drift between imports — Phase 2 (become findings, not failures).
- `import_runs` history carried across rebuilds — Phase 2 (needs "read the previous store", which findings need anyway). Phase 1 column shape must not preclude it.
- `snapshots.keep` pruning and `quarry snapshots` — Phase 2. Note: unbounded snapshot growth (~200 MB per sync) until then.
- `sync.busy_timeout` and any config file — Phase 2.
- Masking account numbers / card patterns in free text — before Phase 3 (first surface where store data can reach Claude). Phase 1 does not import account numbers at all.
- Securities, prices, holdings, FX rates, ACB — Phase 4. Investment accounts (`brokerage`, `retirement`) are imported as accounts but not balance-checked.
- Investment transactions (buys, sells, dividends, share moves: v9 `InvestmentTransaction` rows and their entries) — Phase 4. Phase 1 imports only the cash-flow transactions of brokerage and retirement accounts; their transaction sum is not their balance. Skipped rows are counted (`not_imported`), never silent.
- Tax lines on categories (`ZTAXREFUS` / `ZTAXREFCA` integer codes, no names in the file) — Phase 4 (tax reporting).
- `--from latest`.
- Views, reporting commands, findings, skill, MCP — Phases 2–3.

## Business Rules & Invariants

- **P1-1 Quicken is read-only.** Phase 1 reads the snapshot only; `--from` never opens anything under a Quicken bundle.
- **P1-2 All-or-nothing store.** The store is built into `.quarry-<UTC start>.duckdb.partial` in the store directory and renamed over `quarry.duckdb` only after every check passes. On any failure or interruption the partial is removed and `quarry.duckdb` is byte-identical to before (or still absent). Only one store exists after a swap; no `quarry.duckdb.wal` exists after a swap.
- **P1-3 Balance gate (user ruling 2026-09-28).** Quicken v9 stores no current register balance. For each non-investment account with at least one non-deleted `ZRECONCILERECORD`, the sum of the account's imported transactions with status `reconciled` must equal the `ZENDINGBALANCE` of its newest reconcile record (among non-deleted records: a NULL `ZENDDATE` ranks newest — it refuses with reason 10 rather than falling back to an older, possibly stale statement; otherwise latest `ZENDDATE`; ties → the higher `Z_PK`) to the cent. `statement_date` is that record's `ZENDDATE`; the `quarry` amount is the reconciled sum. The gate verifies reconciled accounts only; never-reconciled accounts are covered by the split and transfer checks alone. Accounts with no reconcile record are counted and listed as never reconciled — not failed, no warning; the `never reconciled` clause counts non-investment accounts only. Investment accounts are counted, not checked. Closed and inactive accounts are checked like any other.
  - Probe on the user's real file (20260928T112701Z): reconciled-status sum matched 6/6 reconciled accounts; "dated ≤ ZENDDATE" matched only 4/6 (dropped). 6 of 25 accounts are reconciled.
  - PRD Q2 "at the latest date" clause needs a PRD amendment: no Quicken source exists for it.
- **P1-4 Split sum.** Every imported transaction has ≥1 split and its splits sum to its amount. `transactions.amount` comes from `ZTRANSACTION.ZAMOUNT` and is never recomputed from splits; split amounts come from `ZCASHFLOWTRANSACTIONENTRY.ZAMOUNT` (via `ZPARENT`) — independent columns in separate tables. Covers imported (cash-flow) transactions only. Probe: 0 zero-entry and 0 mismatching transactions in the real file.
- **P1-5 Transfers.** A transfer is a pair of split legs between the user's own accounts. v9 links them through `ZCASHFLOWTRANSACTIONENTRY.ZTRANSFER`: a numeric value is the counterpart entry's `ZQUICKENID` (symmetric, opposite amounts); a non-numeric value is an account name recorded with no counterpart leg. Each pair is stored once (deduplicated by ordered split ID). Pairing searches every account in the snapshot, including brokerage and retirement accounts, whose transfer legs are cash-flow transactions (a chequing→brokerage contribution is paired, not one-sided). Cross-currency pairs keep both legs at native amounts and are not checked for equal value; `cross_currency` derives from the two accounts' currencies. A leg with no counterpart — a numeric link that resolves to nothing, or any name-form leg — is stored as a one-sided transfer (other leg null, `other_account_id` set when the recorded name matches an account) and warned about (W2), not a failure.
- **P1-5b Row filters.** Rows with `ZDELETIONCOUNT ≠ 0` are excluded from every table, silently. Only `CashFlowTransaction` entity rows (resolved from `Z_PRIMARYKEY` at import, never hard-coded) are imported as transactions; `SmartCashFlowTransaction` (scheduled) rows are never imported; `InvestmentTransaction` rows and their entries are counted into `not_imported.investment_transactions` and skipped. Category tags and user tags are likewise told apart by entity name via `Z_PRIMARYKEY`.
- **P1-5d References (ruled 2026-09-28).** A reference to a deleted row follows the deletion: a transaction in a deleted account, and an entry whose parent transaction is deleted or skipped under this rule, are excluded silently with it (never validated, never counted; `Rows` counts stored rows). A reference to a row that does not exist is treated as NULL: on a required column it refuses with S4 reason 10 verbatim (`a transaction (source id 1234) has no account`, `a split (source id 5678) has no transaction`); on a nullable column it stores NULL (payee, entry category, parent category — `full_path` built from the surviving chain). A `split_tags` link missing either end is not stored. Entry → SmartCashFlowTransaction parent: skipped silently; → InvestmentTransaction parent: counted under `not_imported` (unchanged).
- **P1-5c Dates.** `transactions.date` is `ZPOSTEDDATE`, else `ZENTEREDDATE` (1,017 of the real file's rows have no posted date). Core Data timestamps are seconds since 2001-01-01 UTC; the date is derived in UTC. Phase 2 register matching depends on this rule.
- **P1-6 Validation runs every check.** The build does not stop at the first failing check; the refusal lists every failing account and transaction, no cap.
- **P1-7 Money** is `DECIMAL(18,2)` in native currency; no floats anywhere in the path. Negative = money leaving the account. In `--json`, money is a string with exactly 2 decimals (`"-1204.17"`), dates `YYYY-MM-DD`. Source money columns hold SQLite integer or real values (mixed); quarry reads them as text and parses them exactly — no float arithmetic in quarry. A value with more than 2 decimal places, or outside `DECIMAL(18,2)` range, refuses the import (S4); a money value stored as text or blob refuses with reason 11. Probe: 0 such values in the real file. Source money is read as `typeof(col)` + `CAST(col AS TEXT)` into int64 cents. A REAL-stored value in exponent form or with |v| ≥ 1e13 is refused with reason 6 (SQLite prints REALs to 15 significant digits, so cents above that are untrustworthy); integer-stored values keep the full DECIMAL(18,2) range. IDs are `<prefix>-<Z_PK>` VARCHAR (`acct`, `cat`, `payee`, `tag`, `txn`, `split`); category `full_path` joins names with `:`.
- **P1-8 Stable IDs.** Every row keeps its Quicken `source_id`; quarry IDs are derived deterministically from source IDs, so rebuilding from the same snapshot yields the same IDs.
- **P1-9 Supported values.** Currencies CAD and USD only. A currency, account type or required NULL the importer cannot map refuses the import (S4) rather than guessing.
- **P1-10 One mapping package.** Only the importer knows Quicken's schema; the store package knows only quarry's schema.
- **P1-11 Manifest is a record of the snapshot.** It is never rewritten by the import or by `--from`. `--json` stdout = manifest + top-level `store` key (+ import warnings in `warnings`). Amends Phase 0's "stdout byte-identical to manifest" rule.
- **P1-12 `--from` re-checks the schema against the current reference**, not the one recorded in the manifest; a snapshot whose manifest said `verified: false` imports if the current reference now matches.
- **P1-13 Leftovers.** `.partial` build files and their `.wal` older than 1 hour are removed silently at the start of a later sync (same age gate as snapshot leftovers). A stale `quarry.duckdb.wal` is removed before or at the swap.
- **P1-14 Interrupt.** ctx is checked once before the swap; a signal before it → I2; after it the run completes normally. Phase 0's I1 is unchanged for the snapshot phase.

### Store enum values (permanent SQL surface)
- `accounts.type`: `chequing | savings | credit_card | asset | liability | home_equity | brokerage | retirement`. Map from `ZTYPENAME`: CHECKING→chequing, SAVINGS→savings, CREDITCARD→credit_card, ASSET→asset, LIABILITY→liability, HOMEEQUITY→home_equity, BROKERAGENORMAL / BROKERAGEOTHER→brokerage, DEFERREDCOMPRETIREMENT401K / RETIREMENTIRA→retirement. Any other `ZTYPENAME` → S4 naming it (no speculative `cash` / `loan`). Investment accounts = `brokerage` + `retirement`.
- `accounts.currency`: `CAD | USD`
- `accounts.closed` ← `ZCLOSED`; `accounts.active` ← `ZACTIVE`. v9 has no hidden column; `hidden` is not modelled for accounts.
- `transactions.status`: `uncleared | cleared | reconciled` ← `ZRECONCILESTATUS` NULL or 0 / 1 / 2; any other value → S4.
- `categories.kind`: `income | expense | system` ← `ZTYPE` 2 / 1 / 0; any other value → S4. `system` covers Quicken's built-in categories (Transfer, Uncategorized, Adjustment, Credit Card Payment, investment actions). Deliberate departure from the PRD's income/expense/transfer: transfer exclusion is keyed on `transfers` / `splits.transfer_account_id`, never on category kind.
- `categories.hidden` ← `ZHIDDEN`. PRD "retired kept" dropped: no source; every category is imported anyway.

### Tables (Phase 1)
`accounts` (id, source_id, name, type, currency, institution, closed, active), `categories` (id, source_id, parent_id, name, full_path, kind, hidden), `payees` (id, source_id, name — as recorded, never merged), `transactions` (id, source_id, account_id, date, payee_id, memo, amount, currency, status, cheque_number), `splits` (id, source_id, transaction_id, category_id, amount, memo, transfer_account_id), `transfers` (id, from_split_id, to_split_id nullable, cross_currency), `tags` (id, source_id, name), `split_tags` (split_id, tag_id), `import_runs` (id, started_at, finished_at, snapshot_path, snapshot_sha256, schema_fingerprint, row counts, balances_checked, balances_mismatched, splits_mismatched, transfers_one_sided, investment_transactions_not_imported). Column names are the architect's to finalise within these shapes. Note for Phase 2: `balances_mismatched` / `splits_mismatched` can only be 0 in Phase 1, because a failed build is never stored.

---

## Triage Brief

PRD `docs/initial-prd.md`: milestone `:299-306` ("1 — Import + store | Banking, credit, splits, transfers, categories | Gate: Balances reconcile for all cash accounts"); data model `:110-136`; sync `:140-154`; validation `:271-276`; exit codes `:181-187`; duckdb-go `:104`, `:338`.

**Already exists — do not re-plan:**
- `internal/snapshot/snapshot.go` `Server.Sync` `:97-173` — verified, hashed, schema-checked snapshot + `Manifest` (`manifest.go`). Phase 1 consumes it; does not change snapshot-taking.
- `internal/snapshot/ports.go` — port precedent (consumer-defined interface, unexported production adapters, `export_test.go` fakes). New ports follow it.
- `internal/quicken/v9/reference.go` — `v9.Reference(ctx)`: schema shape only (84 tables), no row semantics. `v9fixture` builds v9 fixture databases.
- `internal/platform/sqlite/sqlite.go` — `Exec`, `Schema`, `IntegrityCheck`, `QueryInt`, `OpenReadOnly*`, `Backup`. No multi-row scan helper yet.
- `internal/platform/atomicfile` — partial+rename precedent.
- `internal/cli/sync.go` `:36-116` — cobra `sync`; RunE stops after `srv.Sync`. `internal/cli/render.go` — output rendering. `cmd/quarry/run.go` `:30-46` `newServer` wires only `snapshot.Server`.
- Mapping prior art: `docs/prior-art/hardkoded/plugins/quicken/skills/quicken-setup/sql/views.sql`, `docs/prior-art/dweekly/schema.md`, `docs/prior-art/dweekly/plugin/skills/quicken/references/balances.md`.

**Must be built:** DuckDB dependency (`duckdb-go`, cgo; absent from `go.mod`); importer package; store package (DuckDB schema + atomic swap); deterministic IDs; reconciliation; `sync` wiring + `--from`.

**Open debts inherited:**
- `buildManifest` reads the snapshot with concrete `sqlite`/`os` calls, no read port (phase0 STATE.md). Architect rules explicitly so the importer and manifest do not grow two ad hoc snapshot readers.
- EDQUOT during build → S2 (covers the build side of the Phase-0 R14c→R14b debt; snapshot-side routing stays a debt unless an architect folds it).
- `cmd/quarry/run_test.go` and `internal/snapshot/sync_faults_test.go` flagged twice for a split "before the next feature adds to either". Phase 1 adds to `run_test.go` — split it first (behaviour-neutral) or put Phase 1's command tests in a new file.
- `--json` stdout gains `store` (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` changes to "stdout minus `store` equals the manifest".
- **Caller table (correcting triage):** under the ruled design `cli.ServerFactory` keeps its shape; sequencing moves into `snapshot.Server`. Caller table recorded under the 01a planning rulings below.

**Architecture rulings for SCENARIO-01a (sizing pass):**
- The rules "skip import on schema mismatch" and "O1 → O1b once the build was reached" are business rules and must not sit in cli's RunE. Either cli calls one feature entry point that sequences snapshot → import, or a consumer-declared port is wired in `cmd`. `Manifest` / `MismatchError` must not cross into the importer unless moved down or adapted in `cmd`. Record the choice in an ADR (`docs/adr/`).
- `internal/store` is a shared lower package (quarry schema DDL, row types, DuckDB builder, swap), so later phases can read it without a feature→feature import. The importer declares its own narrow `Store` port; the store builder implements it. ADR.
- `buildManifest` read-port debt: no port for it. `--from` needs `Manifest` decode + snapshot→manifest resolution as a `snapshot.Server` method reusing `buildManifest`'s integrity / hash / schema steps (extracted, not duplicated). The importer's row reads are a separate concern: its own `Source` port over a new multi-row `platform/sqlite` query helper.
- DuckDB: `github.com/duckdb/duckdb-go/v2` (verified on the module proxy; latest stable `v2.10505.0`). Confine the import to `internal/platform/duckdb` and the store adapter so other test binaries do not link it. `go get` needs network (run unsandboxed). Linux CI linking memory is a known risk (`devenv.nix` notes OOM on linux-small).
- Checkpoint and close DuckDB and assert no `.partial.wal` remains before the rename; renaming with a live WAL orphans committed data.

**Planning rulings from the SCENARIO-01a sizing (architect, 2026-09-28) — binding on later plans:**
- `internal/store` holds only row types, `Rows`, `Counts`/`Result` (no driver import); `internal/store/duckstore` holds DDL, builder and swap and is the only non-platform importer of duckdb-go. Refines the "shared lower package" ruling; the store ADR records it.
- The importer's `Store` port is `Replace(ctx, store.Rows) (path string, err error)`. Validation (01b) runs on mapped `store.Rows` in memory before `Replace`, so a V1 failure never creates a partial.
- Sequencing lives in `snapshot.Server`: a consumer-declared `Importer` port wired with `snapshot.WithImporter`, and a new method returning `Outcome{Manifest, Store *store.Result}` that skips the import on schema mismatch (`Store == nil`). `Sync` keeps its signature. The O1/O1b choice becomes a snapshot function on `Outcome`. `cli.ServerFactory` keeps its shape (caller table: LSP `internal/cli/run.go:14`, `run.go:20`, `root.go:9`, `sync.go:36`, `cmd/quarry/run.go:60`; grep `sync.go:64`, `sync.go:88`). No cli/cmd test builds a factory.
- Import errors do not go through `snapshot.FailureOutcome` (the snapshot is already committed).
- 01a does not count skipped investment rows; `not_imported` and the Rows clause are 01c/S21's. `--json` stays manifest-only until SCENARIO-02.
- Until SCENARIO-14, an unmappable value surfaces through the S3 frame with the ruled S4 reason text; the importer returns a typed error asserted with `errors.As`. SCENARIO-14 moves it to the S4 frame and ordering.
- Help text ownership: 01b owns root Long and sync Short/Long (balance wording is only true after 01b); 03 owns the `--from` paragraph and Example.

**Traps carried into plans:**
- Phase 0 fixtures (`v9fixture.OpenBundle`) insert accounts with only `ZNAME` (NULL type and currency); once import is wired, P1-9 turns every Phase-0 command test into S4. SCENARIO-01d upgrades those fixtures, and `ExtraSchemaBundle` too (non-WAL, passes the bundle check, so import runs on it).
- Until SCENARIO-01b lands, 01a swaps in an unchecked store; 01a's Handoff says so.

**Real-file probe results** (read-only, immutable, SHA unchanged; counts only): see P1-3, P1-4, P1-5, P1-5b, P1-5c, P1-7 and the enum section. Entities: CashFlowTransaction 14,061; InvestmentTransaction 1,605; SmartCashFlowTransaction 0. `ZTRANSFER`: 1,318 numeric (all resolve, symmetric), 29 name-form (2 resolve to an account name). 3 entries deleted.

## Product Verdict

**SHIP WITH CHANGES** (scoping pass), all folded in:
1. Balance gate has no "latest balance" source in v9 → user ruled: last reconciled statement per account (P1-3).
2. `--json` stdout no longer byte-identical to the manifest; manifest never rewritten (P1-11).
3. Phase 0 O1 copy becomes false once a store is built → O1b.
4. Money in `--json` as 2-decimal strings (P1-7).
5. Transfer pairing spans investment accounts (P1-5).
6. Split-sum check needs an independent source (P1-4).
7. Remove stale `quarry.duckdb.wal` (P1-13).
8. Root help text updated.
9. One-sided transfer is a warning (W2), not a failure. Never-reconciled accounts are listed, not warned.

User rulings (2026-09-28): validation = balances + splits + transfers (statement history and drift deferred); `--from` ships; `import_runs` minimal; payees and tags in scope; balance gate = last reconciled statement.

**Scoped ruling on v9 probe findings — SHIP WITH CHANGES**, approved by the user 2026-09-28: balance formula = reconciled-status sum (P1-3); `categories.kind` = income / expense / system, `tax_line` and "retired" dropped, `categories.hidden` kept; `accounts.type` enum per the ZTYPENAME map, account `hidden` replaced by `active`; investment transactions counted not imported; exact decimal parse with S4 on >2dp; name-form transfers are one-sided under W2 with reworded copy; NULL/0 reconcile status = `uncleared`. SCENARIO-05/06/07/08 assertions amended and SCENARIO-21 added accordingly.

## Surface & Copy

Implement verbatim. Paths are `~`-abbreviated in human output and stderr, absolute in `--json`. `<id>` = snapshot basename (e.g. `20260927T143005Z`).

### Command and flags
`quarry sync` only; no new commands.

| Flag | Help string (backticks = pflag placeholder) |
|---|---|
| `--quicken` | unchanged: ``"`path` to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)"`` |
| `--from` (new) | ``"`snapshot` to rebuild the store from instead of reading Quicken: an ID such as 20260927T143005Z, or the path to its .sqlite file"`` |
| `--json` | unchanged |

`--from` resolution: a value containing `/` or ending in `.sqlite` is a path (leading `~/` expanded, as `--quicken`); anything else is an ID → `~/Library/Application Support/quarry/snapshots/<ID>.sqlite`. `--from` never runs discovery, never touches Quicken, never rewrites the manifest.

### Files
| Item | Value |
|---|---|
| Store | `~/Library/Application Support/quarry/quarry.duckdb`, mode `0600` |
| Build file | `~/Library/Application Support/quarry/.quarry-20260927T143005Z.duckdb.partial` (UTC build start) |
| On failure | build file removed |
| Leftovers | `.partial` + its `.wal` older than 1 hour removed silently at next sync start |
| `quarry.duckdb.wal` | never present after a swap |

### Help text
Root Long:
```
quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. quarry never writes to the Quicken file.
```
`sync` Short: `Snapshot the open Quicken file and rebuild quarry's store from it`

`sync` Long:
```
Copy the Quicken file's database with SQLite's backup API into
~/Library/Application Support/quarry/snapshots/, check its integrity, and
compare its tables and columns with quarry's reference for Quicken Classic
for Mac v9. A JSON manifest is written next to each snapshot.

quarry then rebuilds its store, ~/Library/Application Support/quarry/quarry.duckdb,
from the snapshot. In every reconciled account, the reconciled transactions
must add up to the balance of its last reconciled statement in Quicken to the
cent, and every transaction must equal the sum of its splits; if a check
fails, the previous store is left unchanged.

Quicken must be running with the file open: it encrypts the database when
the file is closed. quarry only reads the Quicken file; it never writes to it.

Without --quicken, quarry looks for .quicken files in ~/Documents and in
~/Library/Application Support/Quicken/Documents, and uses the one it finds
if there is exactly one.

With --from, quarry rebuilds the store from a snapshot it took earlier and
does not read Quicken at all.
```
Example:
```
  quarry sync --quicken ~/Documents/Home.quicken
  quarry sync --from 20260927T143005Z
```

### Success (exit 0) — stdout
Phase 0 six-line block unchanged as prefix; five lines appended:
```
Snapshot  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite
Manifest  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.json
Source    ~/Documents/Home.quicken
Size      212.4 MB, 42 accounts
SHA-256   9f86…0a08
Schema    matches reference hardkoded/quicken-skills@752107b (71 tables, 1,042 columns)
Store     ~/Library/Application Support/quarry/quarry.duckdb
Rows      18,204 transactions, 21,977 splits, 3,112 transfers, 1,873 payees, 312 categories, 14 tags
Balances  35 accounts match Quicken's last reconciled balance; 3 never reconciled and 4 investment accounts not checked
Splits    all 18,204 transactions equal the sum of their splits
Transfers 3,112 paired
```
- Rows gains a trailing clause when investment transactions were skipped: `Rows      18,204 transactions, 21,977 splits, 3,112 transfers, 1,873 payees, 312 categories, 14 tags; 1,605 investment transactions not imported` (singular `1 investment transaction`; clause omitted at 0).
- Rows nouns inflect each on its own count: singular at exactly 1 (`1 transaction`, `1 split`, `1 transfer`, `1 payee`, `1 category`, `1 tag`), plural at 0 and N ≥ 2 (`0 transactions` … `0 tags`); counts comma-grouped. Example: `1 transaction, 2 splits, 0 transfers, 1 payee, 3 categories, 0 tags`.
- Transfer counted once per pair. Money on stdout: thousands separators, 2 decimals, leading `-` (`-1,204.17`).
- Balances clauses: zero-count clauses omitted; two clauses joined with ` and `. Singular: `1 account matches Quicken's last reconciled balance`; `1 never reconciled`; `1 investment account not checked` (joined: `1 never reconciled and 1 investment account not checked`). None checkable: `no accounts to check; 3 never reconciled and 4 investment accounts not checked`.
- Splits: exactly 1 → `the 1 transaction equals the sum of its splits`; zero → `no transactions to check`.
- Transfers: none → `none`; with one-sided → `3,112 paired, 3 one-sided`; all one-sided → `0 paired, 2 one-sided`.
- Zero transactions Rows: `0 transactions, 0 splits, 0 transfers, N payees, N categories, N tags`.
- `--from` success: same block; `Source` from the manifest; `Size` and `SHA-256` recomputed, SHA-256 verified against the manifest.
- TTY vs pipe: same bytes.

### `--json` (stdout document)
```json
{
  "snapshot": { …unchanged… },
  "schema":   { …unchanged… },
  "store": {
    "path": "/Users/david/Library/Application Support/quarry/quarry.duckdb",
    "built": true,
    "rows": {"accounts":42,"categories":312,"payees":1873,"tags":14,"transactions":18204,"splits":21977,"split_tags":220,"transfers":3112},
    "balances": {
      "checked": 35,
      "mismatched": [],
      "never_reconciled": [{"id":"…","name":"Wallet","currency":"CAD","closed":false,"active":true}],
      "investment_accounts": 4
    },
    "splits":    {"checked": 18204, "mismatched": []},
    "transfers": {"paired": 3112, "cross_currency": 41, "one_sided": []},
    "not_imported": {"investment_transactions": 1605}
  },
  "warnings": []
}
```
- `not_imported` always present; `investment_transactions` is 0 when none.
- `balances.mismatched[]`: `{"id","name","currency","closed","active","statement_date":"2026-08-31","quarry":"-1204.17","quicken":"-1184.17","difference":"-20.00"}`; `difference` = quarry − quicken.
- `splits.mismatched[]`: `{"id","date","account","currency","payee","amount","splits_total"}`.
- `transfers.one_sided[]`: `{"id","date","account","currency","payee","amount","other_account","other_account_id"}`; `other_account` = the name as recorded, or `null`; `other_account_id` = the matching account's id, or `null` when no account matches.
- All lists always present; sorted by name, or (date, account, id). `id` = quarry's stable ID.
- `store: null` when import not attempted (schema mismatch); `"built": false` when the build ran and a check failed.

### V1 — validation failed (exit 1)
Every check runs. stdout carries the full block, both modes (`"built": false` in json):
```
Store     NOT REBUILT (~/Library/Application Support/quarry/quarry.duckdb unchanged)
Rows      18,204 transactions, …
Balances  DIFFER for 2 of 35 accounts; 3 never reconciled and 4 investment accounts not checked
  ! US Chequing (USD)            2026-08-31  quarry  8,310.00  Quicken  8,300.00  difference  10.00
  ! Visa Infinite (CAD, closed)  2026-07-15  quarry -1,204.17  Quicken -1,184.17  difference -20.00
Splits    DIFFER for 1 of 18,204 transactions
  ! 2024-03-02  Visa Infinite (CAD)  Costco  amount -212.40  splits -202.40
Transfers 3,112 paired
```
- First run (no previous store): `Store     NOT BUILT (no store at ~/Library/Application Support/quarry/quarry.duckdb yet)`; stderr keeps "was not changed".
- Rows: two spaces + `!`; columns padded to widest in block, amounts right-aligned; account label `Name (CUR[, closed][, inactive])`, `inactive` only when open and not active; empty payee `(no payee)`. No cap.

stderr, one line, failing clauses joined with ` and `:
```
quarry: validation failed: 2 of 35 accounts do not match Quicken's last reconciled balance and 1 transaction does not equal the sum of its splits; ~/Library/Application Support/quarry/quarry.duckdb was not changed; each difference is listed on stdout; fix the account in Quicken and run quarry sync, or run quarry sync --from 20260927T143005Z after updating quarry
```
Count form is always `X of Y <noun>`: noun agrees with Y, verb with X — `1 of 1 account does not match`, `1 of 35 accounts does not match`, `35 of 35 accounts do not match`; Splits stdout the same (`DIFFER for 1 of 1 transaction`, `DIFFER for 3 of 3 transactions`); Splits stderr has no denominator. Singular: `1 of 35 accounts does not match`, `1 transaction does not`; plural `N transactions do not equal the sum of their splits`. Snapshot is kept.

### W2 — one-sided transfers (exit 0)
Store built; each stored with other leg null. Covers numeric links that resolve to nothing and every name-form leg. stdout `Transfers 3,112 paired, 29 one-sided` then rows `  ? 2019-06-14  Chequing (CAD)  <payee or (no payee)>  -500.00  other account: <name>`; when the name matches no account: `other account: <name> (not in this file)`; when there is no name at all: `other account: unknown`.
stderr: `quarry: warning: 29 transfers have no matching transaction in another account; quarry keeps them as one-sided transfers`. Singular: `quarry: warning: 1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer`. `warnings[]` holds the text without `quarry: warning: `. W1 prints before W2.

### Refusals (stderr one line; stdout empty unless stated)
| # | Condition | Exact stderr | Exit |
|---|---|---|---|
| F1 | `--from` path does not exist | `quarry: ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite does not exist; check the path passed to --from` | 1 |
| F1b | `--from` ID not found | `quarry: no snapshot 20260927T143005Z in ~/Library/Application Support/quarry/snapshots; check the ID passed to --from` | 1 |
| F2 | not a regular file | `quarry: ~/x is not a snapshot file; pass a .sqlite snapshot from ~/Library/Application Support/quarry/snapshots with --from <snapshot>` | 1 |
| F2b | a `.quicken` bundle | `quarry: ~/Documents/Home.quicken is a Quicken file, not a snapshot; pass it with --quicken <path>, or pass a snapshot with --from <snapshot>` | 1 |
| F3 | not a quarry snapshot | `quarry: ~/x.sqlite is not a quarry snapshot (<reason>); pass a snapshot taken by quarry sync with --from <snapshot>`; `<reason>` ∈ `no .json manifest next to it` / `its manifest is not readable JSON` / `not a SQLite database` | 1 |
| F4 | snapshot or manifest unreadable | `quarry: cannot read ~/x.sqlite: permission denied; check the file's permissions` (OS reason verbatim) | 1 |
| F5 | SHA-256 ≠ manifest | `quarry: ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync` | 1 |
| M1 | schema mismatch, plain sync | unchanged; import not attempted; `store: null`; no Store/Rows/Balances/Splits/Transfers lines | 1 |
| M1b | schema mismatch with `--from` (current reference) | `quarry: schema check failed: snapshot 20260927T143005Z of Home.quicken is missing 1 table and 2 columns that the schema reference expects; quarry cannot import it until its schema reference is updated`; stdout DIFFERS block as M1 | 1 |
| S1 | store dir not writable | `quarry: cannot write to ~/Library/Application Support/quarry: permission denied; make the directory writable by your user` | 1 |
| S2 | ENOSPC / EDQUOT during build | `quarry: cannot write the store to ~/Library/Application Support/quarry: no space left on device; free disk space, then run quarry sync --from 20260927T143005Z` | 1 |
| S3 | other build / DuckDB failure | `quarry: cannot build the store in ~/Library/Application Support/quarry: <reason>; run quarry sync --from 20260927T143005Z` | 1 |
| S4 | unmappable value | `quarry: cannot import snapshot 20260927T143005Z: <reason>; ~/Library/Application Support/quarry/quarry.duckdb was not changed; run quarry sync --from 20260927T143005Z once quarry supports it` — reasons below | 1 |
| I2 | SIGINT/SIGTERM during build, before the pre-swap check | `quarry: sync interrupted while building the store; ~/Library/Application Support/quarry/quarry.duckdb was not changed; run quarry sync --from 20260927T143005Z to rebuild it` | 1 |
| O1b | stdout write fails once the build was reached (replaces O1 then) | `quarry: cannot write the result to stdout: <OS reason>; run quarry sync --from 20260927T143005Z --json to see it again` | 1 |
| U3 | `--from` with `--quicken` | `quarry: --from and --quicken cannot be used together; --from rebuilds from a snapshot without reading Quicken` (custom, not cobra's group message) | 2 |
| U4 | `--from` empty / all whitespace | `quarry: flag needs an argument: --from; Run 'quarry sync --help' for usage.` | 2 |

On a plain sync, S1–S4/I2/V1 keep the already-committed snapshot.

S4 `<reason>` forms (first offender by date, then account, then source id; ` (and N more)` appended when others exist):
1. `account "Euro Savings" uses currency EUR; quarry supports CAD and USD accounts`
2. `account "X" has type ZZZ, which quarry does not map yet`
3. `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`
4. `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`
5. `the 2026-08-31 statement for "Visa Infinite" has a balance of 12.345, which has more than 2 decimal places`
6. `… has an amount of <v>, which is too large for quarry's amounts` (same subjects as 3–4; statement form `the 2026-08-31 statement for "Visa Infinite" has a balance of <v>, which is too large for quarry's amounts`; numeric storage — `integer` / `real` — only: out of range, or REAL in exponent form / |v| ≥ 1e13)

11. Money stored as `text` or `blob` (incl. empty / whitespace; value never quoted — may be bytes, long, or hold card-like digits): `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number` · `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number` · `the 2026-08-31 statement for "Visa Infinite" has a balance that is not a number`; fallback `a transaction in "Visa Infinite" (source id 1234) has an amount that is not a number`. A NULL amount is reason 10, not 11.
7. `the snapshot has no CashFlowTransaction entity, which quarry needs to read Quicken's records`; several: `the snapshot has no CashFlowTransaction, CategoryTag or UserTag entity, which quarry needs to read Quicken's records` (names sorted, joined `, ` … ` or `)
8. `a transaction on 2024-03-02 in "Visa Infinite" has reconcile status 7, which quarry does not map yet`
9. `category "Food:Groceries" has type 5, which quarry does not map yet` (subject = full path)
10. Required NULL, one form `<subject> has no <field>` (field = user-facing noun, never a column name): `account "Chequing" has no currency` · `account "Chequing" has no type` · `an account (source id 42) has no name` · `a transaction (source id 1234) has no account` · `a transaction on 2024-03-02 in "Visa Infinite" has no amount` · `a transaction in "Visa Infinite" (source id 1234) has no date` · `a split of a transaction on 2024-03-02 in "Visa Infinite" has no amount` · `a split (source id 5678) has no transaction` · `category (source id 99) has no name` · `category "Food:Groceries" has no type` · `the 2026-08-31 statement for "Visa Infinite" has no balance` · `a statement for "Visa Infinite" (source id 12) has no date`

Newest statement with NULL `ZENDDATE` and a bad balance (5/6/11 outrank 10): subject `a statement for "Visa Infinite" (source id 12)` — e.g. `… has a balance of 12.345, which has more than 2 decimal places` · `… has a balance of <v>, which is too large for quarry's amounts` · `… has a balance that is not a number`; the missing date (reason 10) surfaces on a later run.

Subjects use the best handle the row has (date, account name, category path); when that handle is the missing value, fall back to `(source id N)` (= Quicken `Z_PK` = store `source_id`).

Required columns (NULL → reason 10): `ZACCOUNT.ZNAME` (name), `.ZTYPENAME` (type), `.ZCURRENCY` (currency); imported `ZTRANSACTION.ZACCOUNT` (account), `.ZAMOUNT` (amount), date = `ZPOSTEDDATE` else `ZENTEREDDATE`, both NULL (date); non-deleted entry `ZAMOUNT` (amount), parent link (transaction); category `ZNAME` (name), `ZTYPE` (type); the newest reconcile record per account (the one the gate uses) `ZENDINGBALANCE` (balance), `ZENDDATE` (date). A required reference that points to no row is treated as NULL (reason 10, same text); one that points to a deleted row is deleted with it (P1-5d). Everything else is nullable and never refuses: payee, memo, cheque number, institution, an entry's category (NULL `category_id`; uncategorized is a Phase 2 finding), `ZRECONCILESTATUS` (NULL = `uncleared`), `ZTRANSFER`, parent category, older reconcile records, and any nullable reference (payee, an entry's category, parent category, tag link) that points to a deleted or missing row — stored as NULL / no link.

Ordering: dated rows by date, account, source id; undated rows (accounts, categories, entity names) by name / full path, then source id. When several classes fail, S4 reports the first class in order 7, 1–6, 11, 8–10 (a missing entity makes row checks meaningless); ` (and N more)` counts offenders of that class only.

Interim frame (ruled 2026-09-28): until SCENARIO-14, S4 cases may surface in the S3 frame, but the importer's typed error carries the `<reason>` verbatim (incl. ` (and N more)`) from 01d on, and tests assert the reason substring (and the type via `errors.As`), not the frame. SCENARIO-14 swaps only the frame. No push or PR may ship with an S4 case in the S3 frame — the final gate treats that as a BLOCKER.

Interim V1 (ruled 2026-09-28): until SCENARIO-09 adds the V1 stdout block, a failed check exits 1 with empty stdout, store untouched, snapshot kept, and stderr is the V1 line with only `each difference is listed on stdout; ` removed, e.g. `quarry: validation failed: 1 of 3 accounts does not match Quicken's last reconciled balance; ~/Library/Application Support/quarry/quarry.duckdb was not changed; fix the account in Quicken and run quarry sync, or run quarry sync --from 20260927T143005Z after updating quarry`. SCENARIO-09 re-inserts the clause and adds stdout; nothing else changes. No push or PR may ship a V1 failure with empty stdout — final-gate BLOCKER.

### Edge-case rows
| Output | Input class | Ruling |
|---|---|---|
| Store line | success | store path |
| | validation failed, previous store | `NOT REBUILT (… unchanged)` |
| | validation failed, first run | `NOT BUILT (no store at … yet)` |
| | schema mismatch | no store lines; `store: null` |
| | schema extras only | after the last `+` row; the Phase 0 block incl. `+` rows is the unchanged prefix; W1 on stderr (W2 after it), exit 0. Same placement for `NOT REBUILT` / `NOT BUILT` under V1 |
| Balances | never reconciled | clause + json list; no warning; exit 0 |
| | investment accounts | clause only |
| | closed/inactive | checked; labelled only in mismatch rows; `closed` and `active` in json |
| | never reconciled clause | counts non-investment accounts only |
| Rows | investment transactions skipped | `; N investment transactions not imported`; json `not_imported.investment_transactions` |
| Transfers | name-form leg, name matches an account | one-sided, W2; `other account: <name>`; `other_account_id` set |
| | name-form leg, no matching account | one-sided, W2; `other account: <name> (not in this file)`; `other_account_id: null` |
| Transfers | cross-currency | silent in human output; json `cross_currency`; both native legs |
| | counterpart in investment account | paired |
| `--from` | `verified: false` manifest, current reference matches | imports |
| `--from` | Quicken closed/absent | irrelevant |
| Warnings | W1 + W2 | W1 then W2; both in `warnings[]`; exit 0 |
| Manifest | any Phase 1 outcome | never rewritten |

---

## Prep (before SCENARIO-01a; not scenarios)

- **PREP-a — split oversized test files (behaviour-neutral, outside the pipeline).** `cmd/quarry/run_test.go` (963 lines) and `internal/snapshot/sync_faults_test.go` (953 lines) split by concern; test count unchanged per `test-stats.py`. Own commit.
- **PREP-c — infrastructure, planned by `architect` and built by `developer` with a checkpoint.** DuckDB dependency (`github.com/duckdb/duckdb-go/v2`); `internal/platform/duckdb` (open, exec, bulk insert, checkpoint/close, error classifiers such as disk-full); a multi-row query helper in `internal/platform/sqlite`; a `v9fixture` row builder seeding `Z_PRIMARYKEY`, accounts, transactions, entries, reconcile records, tags and `Z_15USERTAGS`. Plan file `PREP-c.md`.

---

## Scenarios (Gherkin)

SCENARIO-01 was split at sizing into 01a / 01b / 01c (one `When` each); 01a was split again at planning into 01a (importer + store, acceptance at `importer.Server.Import`) and 01d (`sync` wiring, acceptance through `run()`). Folded scenarios keep their Gherkin; the delivering scenario's acceptance test covers them.

```gherkin
Scenario: SCENARIO-01a — the importer builds a store from a v9 snapshot
  Given a v9 snapshot with cash accounts, split transactions, payees, nested categories and tags
  When the store is built from it
  Then the store receives every account, category, payee, transaction, split, tag and split_tag with native-currency DECIMAL amounts and source_ids

Scenario: SCENARIO-01d — sync imports the Quicken data into a new store
  Given Quicken is open on a file with cash accounts, split transactions, payees, nested categories and tags
  When I run quarry sync
  Then quarry.duckdb holds every account, category, payee, transaction, split, tag and split_tag with native-currency DECIMAL amounts and source_ids
  And stdout shows the Phase 0 block followed by Store and Rows lines, exit 0

Scenario: SCENARIO-01b — sync checks balances and split sums before swapping the store in
  Given Quicken is open on a file whose reconciled accounts match their last reconciled statement and whose transactions equal their splits
  When I run quarry sync
  Then stdout shows the Balances and Splits lines with their counts, and the store is swapped in, exit 0

Scenario: SCENARIO-01c — sync pairs transfers between the user's accounts
  Given Quicken is open on a file with transfers between chequing, savings and credit card accounts
  When I run quarry sync
  Then each pair is one transfers row linking both split legs, and stdout shows "Transfers N paired", exit 0

Scenario: SCENARIO-02 — --json reports the store result alongside the manifest
  Given a sync that builds the store
  When I run quarry sync --json
  Then stdout minus the "store" key equals the snapshot's manifest, and "store" carries path, built, rows, balances, splits, transfers and not_imported with money as 2-decimal strings

Scenario: SCENARIO-03 — rebuild from an earlier snapshot without Quicken
  Given a snapshot 20260927T143005Z taken by quarry and Quicken not running
  When I run quarry sync --from 20260927T143005Z
  Then the store is rebuilt from that snapshot, its manifest is not rewritten, and the output matches a plain sync's

Scenario: SCENARIO-04 — re-importing the same snapshot keeps quarry IDs stable
  Given a store built from snapshot S
  When I rebuild from S again
  Then every row keeps the same quarry ID

Scenario: SCENARIO-05 — category hierarchy and kind are preserved
  Given nested categories with income, expense and system kinds
  When the store is built
  Then each category has its parent_id, full_path, kind and hidden flag

Scenario: SCENARIO-06 — closed and inactive accounts are imported and checked
  Given a closed account and an open inactive account, both reconciled
  When the store is built
  Then both are in accounts with closed and active preserved, and both are counted in Balances

Scenario: SCENARIO-07 — transfers pair, including cross-currency and investment counterparts
  Given a CAD→USD transfer and a chequing→brokerage transfer
  When the store is built
  Then both are paired transfers, the cross-currency one keeps both native amounts, json counts cross_currency
  And the brokerage leg is a cash-flow transaction in the brokerage account

Scenario Outline: SCENARIO-08 — a one-sided transfer is kept and warned about
  Given a transfer leg of kind <leg>
  When I run quarry sync
  Then the store is built with a one-sided transfer row, stdout lists it with "?" and <other>, stderr prints the W2 warning, exit 0
  Examples:
    | leg                                          | other                                        |
    | numeric link whose counterpart is missing    | other account: unknown                       |
    | name-form, name matches an account           | other account: <name>                        |
    | name-form, name matches no account           | other account: <name> (not in this file)     |

Scenario: SCENARIO-09 — a balance mismatch leaves the previous store unchanged
  Given an existing store and an account whose reconciled transactions do not add up to its last reconciled statement's balance
  When I run quarry sync
  Then stdout shows "Store NOT REBUILT" and every mismatched account row, stderr prints the validation-failed line, quarry.duckdb is byte-identical, exit 1

Scenario: SCENARIO-10 — never-reconciled accounts are listed, not failed
  Given an account with no reconcile record
  When I run quarry sync
  Then Balances counts it as never reconciled, json lists it, and the sync succeeds

Scenario: SCENARIO-11 — a transaction whose splits don't sum fails validation
  Given a transaction whose splits total differs from its amount
  When I run quarry sync
  Then Splits DIFFER lists it, the store is not rebuilt, exit 1

Scenario: SCENARIO-12 — a file with no transactions
  Given accounts but zero transactions
  When I run quarry sync
  Then Rows shows 0 transactions, Splits says "no transactions to check", Transfers "none", exit 0

Scenario: SCENARIO-13 — an unmappable value refuses the import
  Given an account in EUR
  When I run quarry sync
  Then stderr prints the S4 refusal naming the account and currency, the store is unchanged, exit 1

Scenario Outline: SCENARIO-14 — a failed build never replaces the store
  Given an existing store
  When the build hits <fault>
  Then stderr prints <refusal>, no .partial file remains, quarry.duckdb is unchanged, exit 1
  Examples:
    | fault                       | refusal |
    | unwritable store directory  | S1      |
    | disk full                   | S2      |
    | other DuckDB error          | S3      |
    | SIGINT before the swap      | I2      |

Scenario Outline: SCENARIO-15 — --from refuses input that is not a usable snapshot
  When I run quarry sync --from <value>
  Then stderr prints <refusal>, nothing is written, exit 1
  Examples:
    | value                                   | refusal |
    | a path that does not exist              | F1      |
    | an unknown ID                           | F1b     |
    | a directory                             | F2      |
    | a .quicken bundle                       | F2b     |
    | no manifest / bad JSON / not SQLite     | F3      |
    | an unreadable file                      | F4      |
    | a snapshot whose hash changed           | F5      |

Scenario Outline: SCENARIO-16 — --from usage errors
  When I run quarry sync <args>
  Then stderr prints <message>, exit 2
  Examples:
    | args                  | message |
    | --from X --quicken Y  | U3      |
    | --from ""             | U4      |

Scenario: SCENARIO-17 — --from re-checks the schema against the current reference
  Given a snapshot whose schema no longer matches the current reference
  When I run quarry sync --from it
  Then stderr prints M1b, stdout shows the DIFFERS block, exit 1

Scenario: SCENARIO-18 — a schema mismatch on plain sync skips the import
  Given a Quicken file whose schema differs from the reference
  When I run quarry sync --json
  Then "store" is null, no store lines are printed, exit 1

Scenario: SCENARIO-19 — each build records an import_runs row
  Given a successful build
  When I query import_runs
  Then one row holds the snapshot hash, schema fingerprint, row counts and reconciliation result

Scenario: SCENARIO-20 — leftovers from earlier builds are cleaned up
  Given a .partial build file older than 1 hour and a stale quarry.duckdb.wal
  When I run quarry sync
  Then both are removed and no quarry.duckdb.wal exists after the swap

Scenario: SCENARIO-21 — investment transactions are counted, not imported
  Given a brokerage account with buy and dividend transactions and a contribution transfer
  When the store is built
  Then only the contribution is in transactions, Rows ends "; N investment transactions not imported", and store.not_imported.investment_transactions is N
```

---

## BDD Acceptance Progress

Order is execution order. Folded scenarios are ticked with the delivering scenario and its acceptance test.

- [x] SCENARIO-01a: the importer builds a store from a v9 snapshot (absorbs 04, 05; test-first: partial + rename) — `internal/importer/import_test.go` `Test_import_builds_every_table_from_a_v9_snapshot`
- [x] SCENARIO-04: re-importing the same snapshot keeps quarry IDs stable — FOLD → 01a, delivered by SCENARIO-01a — `internal/importer/import_test.go` `Test_import_twice_from_the_same_snapshot_keeps_every_id`
- [x] SCENARIO-05: category hierarchy and kind are preserved — FOLD → 01a, delivered by SCENARIO-01a — `internal/importer/import_test.go` `Test_import_keeps_each_categorys_parent_path_kind_and_hidden`
- [x] SCENARIO-01d: sync imports the Quicken data into a new store — `cmd/quarry/run_import_test.go` `Test_run_imports_the_quicken_data_into_a_new_store`
- [x] SCENARIO-01b: sync checks balances and split sums before swapping the store in (absorbs 06) — `cmd/quarry/run_validation_test.go` `Test_run_checks_balances_and_split_sums_before_swapping_the_store_in`
- [x] SCENARIO-06: closed and inactive accounts are imported and checked — FOLD → 01b, delivered by SCENARIO-01b — `internal/importer/validation_test.go` `Test_import_checks_closed_and_inactive_accounts_like_any_other`
- [ ] SCENARIO-09: a balance mismatch leaves the previous store unchanged (absorbs 11)
- [ ] SCENARIO-11: a transaction whose splits don't sum fails validation — FOLD → 09
- [ ] SCENARIO-01c: sync pairs transfers between the user's accounts (absorbs 07, 12, 21)
- [ ] SCENARIO-07: transfers pair, including cross-currency and investment counterparts — FOLD → 01c
- [ ] SCENARIO-12: a file with no transactions — FOLD → 01c
- [ ] SCENARIO-21: investment transactions are counted, not imported — FOLD → 01c
- [ ] SCENARIO-02: --json reports the store result alongside the manifest (absorbs 10, 18)
- [ ] SCENARIO-10: never-reconciled accounts are listed, not failed — FOLD → 02
- [ ] SCENARIO-18: a schema mismatch on plain sync skips the import — FOLD → 02
- [ ] SCENARIO-08: a one-sided transfer is kept and warned about (absorbs 19)
- [ ] SCENARIO-19: each build records an import_runs row — FOLD → 08
- [ ] SCENARIO-14: a failed build never replaces the store (absorbs 13; test-first: write safety)
- [ ] SCENARIO-13: an unmappable value refuses the import — FOLD → 14
- [ ] SCENARIO-20: leftovers from earlier builds are cleaned up (test-first: deletes files; small, sonnet architect)
- [ ] SCENARIO-03: rebuild from an earlier snapshot without Quicken (absorbs 16)
- [ ] SCENARIO-16: --from usage errors — FOLD → 03
- [ ] SCENARIO-15: --from refuses input that is not a usable snapshot (absorbs 17)
- [ ] SCENARIO-17: --from re-checks the schema against the current reference — FOLD → 15
