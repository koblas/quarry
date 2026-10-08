# Specification: Skip rows whose owner is absent

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` imports a real Quicken library that contains a stray Core Data row with no owner — an entry with no parent transaction, or a transaction with no account — instead of refusing the whole import over a row Quicken never shows and the user cannot find or delete.

**Out of Scope**: counting or reporting skipped rows; any new output, `--json` field or exit code; nullable references (payee, entry category, parent category, tag links — unchanged); Smart/Investment parent handling (unchanged).

**Business Rules**: amends P1-5d in `docs/specifications/phase1-import-store/specification.md:31` (and :16, :303 + paragraph after).

## Business Rules & Invariants
- R1: A row whose owner is absent is skipped silently with everything under it — never validated, never counted; `Rows` counts stored rows.
- R2: An owner is absent when it is deleted, when the reference is NULL, or when it points to a row that does not exist. Owner of a transaction = its account; of an entry = its parent transaction. Also absent when the parent is skipped under this rule or refused as its own offender.
- R3: Any amount — no zero-only special case.
- R4: No special case for a skipped entry's transfer link. An imported leg whose transfer link names a skipped entry is one-sided under W2 (existing copy).
- R5: The guard is P1-4: a real transaction that lost its entry still fails V1 (`checkSplits` requires at least one split summing to the amount).
- R6: Reason 10 forms `a transaction (source id N) has no account` and `a split (source id N) has no transaction` become unreachable and are deleted from code and spec.
- R7: No new stdout/stderr line, no `--json` field, no Phase 2 cleanup finding, exit codes unchanged.

---

## Triage Brief

- Reproduced from user snapshot `20260929T154207Z` (read-only): ZCASHFLOWTRANSACTIONENTRY Z_PK 10138, ZPARENT NULL, ZAMOUNT 0, ZCATEGORYTAG 108, ZDELETIONCOUNT 0, created 2022-07-13. Only non-deleted entry in the file with NULL or dangling ZPARENT. Refused by `mapSplits` with reason 10.
- Inconsistency today: P1-5d skips an entry silently when its parent is deleted/Smart/excluded, but refuses when the parent is missing.

**Production inventory (grep; symbols unexported, file-local):**
- `internal/importer/reasons.go:65-68` `reasonTransactionNoAccount`, `:81-84` `reasonSplitNoTransaction` — delete.
- `internal/importer/splits.go:46-53` — two `off.add` sites → silent skip (NULL parent and `txns` miss). `:19-28` doc + `existingTransactions` param drop.
- `internal/importer/transactions.go:109-116` — two `off.add` sites → silent skip. `:91` `existingAccounts` param drops; doc `:81-88` reworded. `surveyTransactions` `:52-79`: `existing` map (`:57,:66-68,:78`) dead, return shrinks; `Z_PK` in `transactionSurveyQuery` (`:50`) then unused.
- `internal/importer/importer.go:64,85,90,94` — `existingAccounts` / `existingTransactions` plumbing drops.
- `internal/importer/accounts.go:48-54,57,67,110` `mapAccounts` — third return `existing` dead; stale doc `:48-51`.
- `existingCategories`, `existingPayees` stay (still read).
- No reader of the two reasons in `cmd/**`, `internal/snapshot`, `internal/cli`. `internal/snapshot/{from.go:155-156,content_refusal.go:31,...}` and `cmd/quarry/run_bundle_refusals_test.go:151` say `has no accounts` (phase0 R12, ZACCOUNT empty) — different path, leave alone.

**Tests to flip:**
- `internal/importer/transactions_test.go:326-334` `Test_import_refuses_a_split_with_no_transaction` → S1 (amount 0).
- `internal/importer/dangling_references_test.go:65-78` `Test_import_refuses_a_split_whose_parent_transaction_does_not_exist` → S2.
- `internal/importer/transactions_test.go:196-206` `Test_import_refuses_a_transaction_with_no_account` → S3 NULL form; `dangling_references_test.go:32-45` `Test_import_refuses_a_transaction_whose_account_does_not_exist` → S3 nonexistent form.
- `internal/importer/coverage_test.go:97-108` `Test_import_skips_a_split_tag_link_whose_split_was_itself_skipped` → `NoError`, no split_tags stored.
- `internal/importer/offenders_internal_test.go:39-49` — literal `a split (source id 1) has no transaction` re-pointed to a live reason-10 form.
- New: S4 in `internal/importer/transfers_test.go` (near `:259`); S5 (`store.ErrValidationFailed`, V1 wording `1 transaction does not equal the sum of its splits`).
- Fixture: `internal/quicken/v9/v9fixture/builder.go:65-72` — `Parent: 0` / `Account: 0` write NULL.

**Regression guards (unchanged, in narrow loop):** `dangling_references_test.go:81-113` (Smart parent), `coverage_test.go:80-95`, `transfers_test.go:159,195-200,259-262,291`, `cmd/quarry/run_validation_test.go:353`, `cmd/quarry/run_transfers_test.go:269`, `internal/snapshot/sync_and_import_test.go:446,474`.

**Already exists — do not re-plan:** `pairTransfers` (`internal/importer/transfers.go:23-80`) builds only from stored splits (one-sided on unresolved numeric link); `checkSplits` (`validate.go:164-166`); silent parent-skip path via `txns` miss in `mapSplits`.

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-09-29): one rule for both ownership edges (entry→transaction, transaction→account); NULL and dangling alike; any amount; silent; delete the two reason-10 forms.

**P1-5d replacement** (`phase1-import-store/specification.md:31`), whole bullet:

> **P1-5d References (ruled 2026-09-28; amended 2026-09-29).** A row whose owner is absent is not part of any register and is skipped silently with everything under it (never validated, never counted; `Rows` counts stored rows). A row's owner is absent when the owner is deleted, when the reference is NULL, or when it points to a row that does not exist. For a transaction the owner is its account; for an entry it is its parent transaction. The same applies when the parent is skipped under this rule or refused as its own offender. Quicken never shows such rows, so they cannot affect a balance. A real transaction that lost an entry still fails P1-4. An imported leg whose transfer link names a skipped entry is one-sided under W2 (existing copy). A nullable reference (payee, entry category, parent category) that is deleted or points to no row stores NULL; `full_path` is built from the surviving chain. A `split_tags` link missing either end is not stored. An entry whose parent is a SmartCashFlowTransaction is skipped silently. An entry whose parent is an InvestmentTransaction is counted under `not_imported` (unchanged).

**Also:**
- `:16` "Skipped rows are counted (`not_imported`), never silent" → "Skipped investment transactions are counted (`not_imported`), never silent."
- `:303` reason 10 list: remove `a transaction (source id 1234) has no account` and `a split (source id 5678) has no transaction`.
- Paragraph after `:303`: drop `imported ZTRANSACTION.ZACCOUNT (account)` and `parent link (transaction)` from required columns; replace "A required reference that points to no row is treated as NULL (reason 10, same text); one that points to a deleted row is deleted with it (P1-5d)" with "An ownership reference (a transaction's account, an entry's parent transaction) that is NULL or points to no row is skipped silently with its row, like one that points to a deleted row (P1-5d)." Nullable-reference half and `(source id N)` fallback stay.
- Add the edge-case rows below to phase1's edge-case table.

## Surface & Copy

No new surface. No new command, flag, output line, warning, exit code or `--json` field. Removed copy: reason 10 forms `a transaction (source id N) has no account`, `a split (source id N) has no transaction`. Existing W2 one-sided copy and V1 `1 transaction does not equal the sum of its splits` used verbatim.

| Output | Input class | Ruling |
|---|---|---|
| sync (store) | entry with NULL parent, amount 0 | imported without it; exit 0; no stderr; Rows excludes it; `--json` unchanged |
| sync (store) | entry with NULL parent, nonzero amount | same |
| sync (store) | entry whose parent points to no row | same |
| sync (store) | transaction with NULL account, or account pointing to no row | transaction and its entries skipped; exit 0; no stderr; Rows excludes them |
| Transfers | imported leg whose numeric link targets a skipped entry's `ZQUICKENID` | existing one-sided W2 row; counted in `M one-sided`; exit 0 with W2 warning |
| sync (store) | skipped entry with `split_tags` rows | links not stored |
| Validation (P1-4) | real transaction whose entry points elsewhere / is gone | existing V1 split-sum failure, exit 1 |

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — A split with no parent transaction is skipped
  Given a snapshot with a non-deleted split whose parent reference is NULL, amount 0
  When the user runs quarry sync
  Then it exits 0, the split is not stored, Rows excludes it, and nothing is printed about it

Scenario: SCENARIO-02 — A split whose parent points to no row is skipped
  Given a snapshot with a non-deleted split whose parent reference names a transaction that does not exist, amount 12.34
  When the user runs quarry sync
  Then it exits 0, the split is not stored, and nothing is printed about it

Scenario: SCENARIO-03 — A transaction with no account is skipped with its splits
  Given a snapshot with a transaction whose account reference is NULL or names no account, with one split
  When the user runs quarry sync
  Then it exits 0, neither the transaction nor its split is stored, and nothing is printed about them

Scenario: SCENARIO-04 — A transfer leg linked to a skipped split is one-sided
  Given a snapshot with an imported split whose transfer link names the Quicken id of a parentless split
  When the user runs quarry sync
  Then the transfer is stored one-sided and the existing one-sided transfer warning is shown

Scenario: SCENARIO-05 — A real transaction missing a split still fails validation
  Given a snapshot with a transaction whose only split points to a different, nonexistent parent
  When the user runs quarry sync
  Then it exits 1 with the existing "equals the sum of its splits" validation failure
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (absorbs 02-05) — ~3 Build batches, 1 package `internal/importer`, ~25 lines removed/collapsed; recommended `code-first` (spec amendment, no write-safety code) |
| SCENARIO-02 | FOLD into SCENARIO-01 — same `splits.go` collapse; test-only fixture flip |
| SCENARIO-03 | FOLD into SCENARIO-01 — ~6 lines in `transactions.go` |
| SCENARIO-04 | FOLD into SCENARIO-01 — no production code; green once 01 lands |
| SCENARIO-05 | FOLD into SCENARIO-01 — no production code; green once the dangling-parent branch is removed too; mutation: skip drops the transaction as well |

Dead after change: `existingAccounts`, `existingTransactions` and the `existing` maps building them — delete, don't leave unread params.

## BDD Acceptance Progress
- [x] SCENARIO-01: A split with no parent transaction is skipped — `internal/importer/splits_test.go` `Test_import_skips_a_split_with_no_parent_transaction`
- [x] SCENARIO-02: A split whose parent points to no row is skipped — delivered by SCENARIO-01 — `internal/importer/splits_test.go` `Test_import_skips_a_split_whose_parent_transaction_does_not_exist`
- [x] SCENARIO-03: A transaction with no account is skipped with its splits — delivered by SCENARIO-01 — `internal/importer/transactions_test.go` `Test_import_skips_a_transaction_with_no_account_and_its_split`
- [x] SCENARIO-04: A transfer leg linked to a skipped split is one-sided — delivered by SCENARIO-01 — `internal/importer/transfers_test.go` `Test_import_keeps_a_link_to_a_skipped_split_as_one_sided`
- [x] SCENARIO-05: A real transaction missing a split still fails validation — delivered by SCENARIO-01 — `internal/importer/validate_test.go` `Test_import_fails_validation_when_a_transaction_lost_its_only_split`
