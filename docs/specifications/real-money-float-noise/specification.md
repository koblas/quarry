# Specification: Import REAL money carrying float residue as its cent

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` imports a real Quicken library whose REAL-stored money values carry float residue a hair off a whole cent (e.g. `-55396.139999999992`, `-15.67000000001`), instead of refusing the whole import over a value Quicken itself shows and sums as the cent.

**Out of Scope**: integer-stored money (unchanged, full DECIMAL(18,2) range); raising `realIntBound`; any new output, warning, or `--json` field; configurable tolerance.

**Business Rules**: amends P1-7 in `docs/specifications/phase1-import-store/specification.md:34`.

## Business Rules & Invariants
- R1: A REAL-stored money value within 0.000001 (inclusive) of a whole cent imports as that cent.
- R2: The distance is computed exactly on the decimal text (`CAST(col AS TEXT)`) — digit strings or exact rationals (`math/big`), never `ParseFloat` / float arithmetic.
- R3: Snapping is silent: no stdout/stderr line, no warning code, no `--json` field.
- R4: One rule in `parseMoney`, applied alike to transaction, split and statement amounts.
- R5: Negative-exponent text (magnitude below 1e-4) with magnitude ≤ 0.000001 imports as 0 cents (`5.5511151231257827e-17` → 0.00); otherwise reason 3/4/5 (`1.0e-05`, `2.0e-06`). Reverses phase1 round-2 ruling.
- R6: Any other value with more than 2 decimal places refuses with reason 3/4/5, quoted as rendered (copy unchanged).
- R7: Integer part ≥ 1e9, positive exponent, or snapped |cents| ≥ 1e11 → reason 6 (`999999999.9999999` → reason 6).
- R8: Snapped zero is 0 cents — no signed zero (`-0.0000001` → 0).
- R9: Snapped values go through P1-3/P1-4/P1-5 validation unchanged; a snap that changed money fails V1.

---

## Triage Brief

- Error reproduced from user snapshot `20260929T145056Z` (sqlite 3.53.4). `internal/importer/money.go` `parseRealMoney` refuses per P1-7 "never rounded"; P1-7's "Probe: 0 such values in the real file" is falsified.
- Offenders across ~24k REAL money values: exactly 2 transactions + their 2 mirror splits, HELOC:
  - ZTRANSACTION 6713 / split 6930, 2020-04-13: `-55396.139999999992` — 1 ULP, 7.9e-12 from cent.
  - ZTRANSACTION 7375 / split 7593, 2020-04-14: `-15.67000000001` — 5630 ULP, 1.0e-11 from cent (genuine Quicken residue).
  - Statement balances: 0 offenders.
- `parseMoney` callers (LSP + grep): `internal/importer/transactions.go:142`, `splits.go:64`, `statements.go:112`; tests `money_internal_test.go:55,86`. No `cmd/**` caller. `moneyPrecision` consumers `transactions.go:156`, `splits.go:69`, `statements.go:121`. Reason copy `reasons.go:31,39,95,99`.
- Tests pinning current refusals: `money_internal_test.go:31` (`12.345`, stays), `:33` (`5.55e-17` → precision, **flips**), `:34` (`1.0e-05`, stays); `transactions_test.go:53-54` (e-17 refusal case, **moves**), `:57-58` (`1.0e-05`, stays); `transactions_test.go:25,177,193`, `statements_test.go:190,232` (`12.345`, stay).
- Spec/doc text to amend: P1-7 (`phase1-import-store/specification.md:34`), `phase1-import-store/STATE.md:30`, `REVIEW-02.md` Rulings 2 (supersession note), `SCENARIO-01a.md:45` (already stale); `money.go` doc comments `:19-24`, `:26-31`, `:53-54`, `:62`; `offenders.go:20-22`; `splits.go:15-22`.

**Already exists — do not re-plan**: `parseMoney`, `parseRealMoney`, `moneyFault`, `realIntBound`, `dollarBound` (`money.go`); reason 3/4/5/6 copy (`reasons.go`); scan test `Test_parseMoney_reads_every_real_just_below_the_bound_exactly` (`money_internal_test.go:68`); fixture `v9fixture.TransactionRow{Amount: <string>}` (`transactions_test.go:53`), `v9fixture.ReconcileRow{EndingBalance: ...}` (`statements_test.go:31`).

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-09-29). Accepted changes:
- Tolerance absolute 0.000001 inclusive, not 1e-9 (1 ULP at 1e9 ≈ 1.2e-7 — 1e-9 would re-create the bug above ~$5M), not relative.
- Exact decimal comparison on text; P1-7's "no float arithmetic" stands.
- Named constant in `money.go`, not config.
- Silent; no new copy.
- Post-snap bound |cents| < 1e11 else reason 6.
- `5.55e-17` → 0.00 (reverses round 2); `1.0e-05` still refuses.
- P1-7 replacement text below (Sweep step).

**P1-7 replacement** — in `phase1-import-store/specification.md:34`, replace from "A value with more than 2 decimal places" through "integer-stored values keep the full DECIMAL(18,2) range." with:

> A value outside `DECIMAL(18,2)` range refuses the import (S4); a money value stored as text or blob refuses with reason 11. Source money is read as `typeof(col)` + `CAST(col AS TEXT)` into int64 cents. **Float residue (ruled 2026-09-29, supersedes round 2 and "never rounded").** A REAL-stored value within 0.000001 (inclusive) of a whole cent imports as that cent, silently (no warning, no `--json` field); the distance is computed exactly on the decimal text (digit strings or exact rationals, never floats), and applies alike to transaction, split and statement amounts. Any other value with more than 2 decimal places refuses with reason 3/4/5, value quoted as rendered. Probe (20260929T145056Z): 2 transactions + their 2 mirror splits (HELOC, 2020-04-13/14: `-55396.139999999992`, `-15.67000000001`) of ~24k REAL money values carry residue; 0 statement balances; residue ≤ 1.0e-11, tolerance 1e-6 keeps ≥ 8 ULP over the whole accepted range (< 1e9) and stays 1000x below a mill. Snapped values are validated by P1-3/P1-4/P1-5 like any other; a snap that changes money fails V1, never passes silently. Exponent form with a negative exponent is a magnitude below 1e-4: ≤ 1e-6 imports as 0.00 (`5.5511151231257827e-17`), otherwise reason 3/4/5 (`1.0e-05`). A REAL-stored value with integer part ≥ 1e9, snapped |cents| ≥ 1e11, or in exponent form with a positive exponent, is refused with reason 6 (ruled at final gate 2026-09-29: SQLite 3.53 prints REALs with up to 17 significant digits; above 1e9 a 2-decimal REAL can render with float noise, so quarry cannot trust its cents — pinned by an in-repo scan test; raising the bound needs a scan proving the new range clean). `0.30000000000000004` imports as 0.30. Integer-stored values keep the full DECIMAL(18,2) range; the tolerance never applies to them.

Also: update `phase1-import-store/STATE.md:30` money bullet; one-line note in `REVIEW-02.md` Rulings 2 "superseded by P1-7 (2026-09-29)".

**Required unit rows** (`Test_parseMoney`): `-55396.139999999992`→-5539614, `-15.67000000001`→-1567, `0.30000000000000004`→30, `12.340001`→1234 (inclusive boundary), `12.3400011`→moneyPrecision, `12.339999`→1234, `-0.0000001`→0, `4.0e-07`→0, `2.0e-06`→moneyPrecision, `999999999.9999999`→moneyTooLarge; `5.5511151231257827e-17`→0 moneyOK; `1.0e-05`→moneyPrecision unchanged.

**Mutation checks**: `≤`→`<` (`12.340001`); tolerance 1e-9 (`12.340001`) and 1e-5 (`2.0e-06`, `12.3400011`); drop post-snap bound (`999999999.9999999`); float comparison.

## Surface & Copy

No new surface. No new command, flag, output line, warning, exit code, or `--json` field. Existing reason 3/4/5/6 copy (`internal/importer/reasons.go`) unchanged and used verbatim. Exit codes unchanged (0 success, 1 refusal).

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — Transaction amounts carrying float residue import as their cent
  Given a snapshot whose HELOC transactions and mirror splits hold REAL amounts -55396.139999999992 and -15.67000000001
  When the user runs quarry sync
  Then it exits 0, the store holds -55396.14 and -15.67, and no warning line is printed

Scenario: SCENARIO-02 — A near-zero residue imports as zero
  Given a snapshot with a transaction whose REAL amount renders 5.5511151231257827e-17
  When the user runs quarry sync
  Then it exits 0 and the store holds 0.00 for that transaction

Scenario: SCENARIO-03 — A statement balance carrying float residue imports as its cent
  Given a snapshot with a statement whose REAL balance is within 0.000001 of a whole cent
  When the user runs quarry sync
  Then it exits 0 and the store holds that cent as the balance

Scenario: SCENARIO-04 — A sub-cent amount beyond the tolerance still refuses
  Given a snapshot with a transaction whose REAL amount is 12.3400011
  When the user runs quarry sync
  Then it exits 1 with reason 3 quoting "12.3400011" and the store is not changed

Scenario: SCENARIO-05 — A residue that rounds to the $1e9 bound refuses as too large
  Given a snapshot with a transaction whose REAL amount renders 999999999.9999999
  When the user runs quarry sync
  Then it exits 1 with reason 6 and the store is not changed
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (absorbs 02-05) — 3 Build batches (snap core, exponent branch, post-snap bound), 1 package `internal/importer`; `Cadence: test-first` (bug fix) |
| SCENARIO-02 | FOLD into SCENARIO-01 — exponent branch, ~10 lines + table row; moves `money_internal_test.go:33`, `transactions_test.go:53-54` |
| SCENARIO-03 | FOLD into SCENARIO-01 — 0 production lines; green on arrival once 01's core lands (`statements.go:112` shares `parseMoney`) |
| SCENARIO-04 | FOLD into SCENARIO-01 — 0 production lines; refuses today, regression guard |
| SCENARIO-05 | FOLD into SCENARIO-01 — ~3 lines post-snap bound; today refuses as reason 3, not 6 |

Proposed runs: A (acceptance red for 01 + folded tests) | B1 (snap core + unit rows) | B2 (exponent branch + post-snap bound) | V. P1-7 amendment is a Sweep doc step.

## BDD Acceptance Progress
- [ ] SCENARIO-01: Transaction amounts carrying float residue import as their cent
- [ ] SCENARIO-02: A near-zero residue imports as zero
- [ ] SCENARIO-03: A statement balance carrying float residue imports as its cent
- [ ] SCENARIO-04: A sub-cent amount beyond the tolerance still refuses
- [ ] SCENARIO-05: A residue that rounds to the $1e9 bound refuses as too large
