# real-money-float-noise — current state

Scenarios complete: SCENARIO-01..05 (02-05 folded into 01). Last updated by SCENARIO-01.

## Binding decisions
- Distance to the cent is exact (`big.Rat` on the decimal text), never `ParseFloat`; tolerance `snapToleranceInverse` = 1e6 (1e-6, inclusive) beside `realIntBound` in `internal/importer/money.go` (SCENARIO-01)
- `parseMoney` is the single snap point; `transactions.go`, `splits.go`, `statements.go` inherit it unchanged; snap is silent — no warning, reason or `--json` field (SCENARIO-01)
- Order in `realCents`: integer part >= 1e9 -> `moneyTooLarge` before the precision test; snapped |cents| >= `realIntBound*100` -> `moneyTooLarge` after it (`999999999.9999999` -> reason 6) (SCENARIO-01)
- Negative-exponent text snaps the same way (<= 1e-6 -> 0 cents, else `moneyPrecision`); positive exponent stays `moneyTooLarge`; snapped zero is `0`, never signed (SCENARIO-01)
- Integer-stored money never goes through the tolerance — `parseIntegerMoney` untouched (SCENARIO-01)
- P1-7 amended in `phase1-import-store/specification.md:34`; `phase1-import-store/STATE.md:30` updated; `REVIEW-02.md` Rulings 2 marked superseded (SCENARIO-01)

## Left unbuilt
- Configurable tolerance, raised `realIntBound`, snap warning — out of scope per spec (SCENARIO-01)

## Traps
- Fixture `Amount` binds as text into a REAL-affinity column, so the rendered text (`1.0e-05`, `999999999.9999999`) is SQLite's, not the literal — assert the text from a red run (SCENARIO-01)
- `Test_parseMoney_reads_every_real_just_below_the_bound_exactly` needs the digit-string fast path for <= 2 decimals; routing all through `big.Rat` slows the 1e6 scan (SCENARIO-01)
- `big.Rat.SetString` accepts `1/3`, `0x…`; only call it after the digit/exponent grammar guards. It rejects huge exponents (`1.0e-99999999999`), which `snapToCent` reports as beyond tolerance -> `moneyPrecision` (Go-version-dependent limit; row `real negative exponent too large for exact arithmetic`) (SCENARIO-01)

## Mutation proofs (money.go, each restored)
- `≤`→`<` reddens `12.340001`; tolerance 1e-9 reddens `12.340001`; tolerance 1e-5 reddens `2.0e-06`, `12.3400011`; dropping the post-snap bound reddens `999999999.9999999` rows and acceptance 05; dropping the `!ok` guard in `snapToCent` panics the huge-exponent row (SCENARIO-01)

## Open debts
- None.
