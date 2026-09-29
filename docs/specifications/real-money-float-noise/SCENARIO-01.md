---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Transaction amounts carrying float residue import as their cent (absorbs 02-05)

Cadence: test-first — bug fix (`build.md` → *Build cadence*: `parseRealMoney` refuses a value Quicken sums as a cent)
Acceptance test: `internal/importer/transactions_test.go` `Test_import_imports_a_transaction_and_split_carrying_float_residue_as_their_cent`
Acceptance test (SCENARIO-02, folded): `internal/importer/transactions_test.go` `Test_import_imports_a_near_zero_residue_amount_as_zero_cents`
Acceptance test (SCENARIO-03, folded): `internal/importer/statements_test.go` `Test_import_imports_a_statement_balance_carrying_float_residue_as_its_cent`
Acceptance test (SCENARIO-04, folded): `internal/importer/transactions_test.go` `Test_import_refuses_an_amount_beyond_the_snap_tolerance`
Acceptance test (SCENARIO-05, folded): `internal/importer/transactions_test.go` `Test_import_refuses_a_residue_that_rounds_up_to_the_bound_as_too_large`
Narrow loop: `go test ./internal/importer/ -run 'parseMoney|residue|tolerance|near_zero|exponent|more_than_2_decimal'`
Mutation checks: `≤`→`<` in the distance test → `Test_parseMoney` row `12.340001`; tolerance 1e-9 → row `12.340001`; tolerance 1e-5 → rows `2.0e-06`, `12.3400011`; drop post-snap bound → row `999999999.9999999` (+ acceptance 05); float comparison (`ParseFloat` distance) → whichever `Test_parseMoney` boundary row reddens (try `12.340001`, `-12.340001`; add one if none does)
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`internal/importer`); FOLDs 02-05 per spec `## Sizing`

## Implementation Plan

Exact decimal: `math/big.Rat` — `SetString` reads decimals and `e-17` text exactly, so distance-to-cent is one exact compare; the 2-digit fast path stays digit-string (keeps the 1e6-value scan test fast). No `strconv.ParseFloat`. No new port, no `Server` option, no `cmd/`/`cli` change: surface is unchanged (spec `## Surface & Copy`).

### Acceptance (red)
- [x] Step 1: `transactions_test.go` (new tests after `:80`; pattern of `:14-27`, assert on `fakeStore.Rows`) — 01: HELOC-style transactions `-55396.139999999992` and `-15.67000000001`, each with a mirror `b.Entry`, `require.NoError`, `Rows.Transactions` amounts -5539614 / -1567 and matching `Rows.Splits`; 02: amount `5.5511151231257827e-17` → NoError, amount 0; 04: `12.3400011` → reason 3 quoting `12.3400011`, `Rows` empty (control: `12.340001` imports); 05: `999999999.9999999` → reason 6 quoting the text as SQLite renders it (confirm rendered text in the red run). Remove the e-17 case from the refusal table `:52-55` (moves to 02); `:56-63` stay.
- [x] Step 2: `statements_test.go` (after `:41`, pattern of `Test_import_uses_the_newest_statement_by_date` with `chequingWithOneReconciledTxn(b, "100.00")`) — 03: `EndingBalance: "100.0000004"` → NoError, `Validation.Balances.Checked == 1`, `Mismatched` empty. Run `A`: 01/02/03/05 red at their assertions (refuse today; 05 as reason 3 not 6); 04 green on arrival (regression guard) — say so. Signature stubs: none needed, no new symbol.

### Build
- [x] Step 3: `money.go:53-85` `parseRealMoney` + `money.go:19-24` + `money_internal_test.go:23-50` `Test_parseMoney` — snap core (B1). Named constant for the 0.000001 tolerance beside `realIntBound`. When `len(fracPart) > 2` and no exponent: exact `big.Rat` distance to nearest cent (half-up), within tolerance inclusive → that cent, else `moneyPrecision`. Integer part ≥ 1e9 → `moneyTooLarge` checked before the precision test (R7). Snapped zero is `0`, never negative (R8). Test-first rows: `-55396.139999999992`→-5539614, `-15.67000000001`→-1567, `0.30000000000000004`→30, `12.340001`→1234 (inclusive edge), `12.3400011`→moneyPrecision, `12.339999`→1234, `-0.0000001`→0, `12.345`→moneyPrecision stays. Bound tests: inside/just outside tolerance, both signs (`-12.340001`, `-12.3400011`).
- [x] Step 4: `money.go:61-67` exponent branch + `Test_parseMoney` rows `:32-34` — (B2) positive exponent stays `moneyTooLarge`; negative exponent: validate mantissa (`digits(.digits)`) and exponent (`-digits`) else `moneyNotANumber`, then the same `big.Rat` snap (magnitude ≤ 1e-6 → 0 cents, else `moneyPrecision`). Flip `:33` to `5.5511151231257827e-17`→0 `moneyOK`; add `4.0e-07`→0, `2.0e-06`→moneyPrecision; `1.0e-05`→moneyPrecision (`:34`, unchanged). Add one malformed-mantissa row (`x.1e-05`→moneyNotANumber).
- [x] Step 5: `money.go` post-snap bound — (B2) snapped |cents| ≥ `realIntBound*100` → `moneyTooLarge`. Rows: `999999999.9999999`→moneyTooLarge, `-999999999.9999999`→moneyTooLarge, control `999999999.99`→99999999999 `moneyOK` (`:35`), `999999999.9999`→moneyPrecision (1e-4 from the bound, outside tolerance).

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments to true up (shorter, no spec ids, no history): `money.go:19-24`, `:26-31` (`parseMoney` — "never through float arithmetic" stays, add the snap), `:53-54`, inline `:62`; `offenders.go:20-22` (precision classes, "more than 2 decimals beyond the snap tolerance"); `splits.go:15-22` (`mapSplits` "too much precision"). Docs: `phase1-import-store/specification.md:34` — replace from "A value with more than 2 decimal places" through "integer-stored values keep the full DECIMAL(18,2) range." with the P1-7 replacement text in this spec's `## Product Verdict`, verbatim; `phase1-import-store/STATE.md:30` money bullet — replace the negative-exponent sentence ("round 2") and "REAL text must be…" clause with the snap rule, and the mutation list; `phase1-import-store/REVIEW-02.md:31` Rulings 2 — append "superseded by P1-7 (2026-09-29)". Leave `SCENARIO-01a.md:45` alone (audit history).

### Verify
- [ ] Step 7: full verification per `agent-briefs.md` → *Verification*; `.claude/scripts/spec-check.py real-money-float-noise`; tick SCENARIO-01..05 in `specification.md` — 02..05 lines read `delivered by SCENARIO-01 — ` before the test reference (test reference last, per `build.md` → *Scenario traceability*); create `docs/specifications/real-money-float-noise/STATE.md`; set `status: done`.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Distance to the cent is computed exactly (`big.Rat`), never `ParseFloat` — P1-7's "no float arithmetic" and R2.
- `parseMoney` is the single snap point; `transactions.go:142`, `splits.go:64`, `statements.go:112` are unchanged and inherit it (R4). No new reason, warning or `--json` field (R3).
- Integer part ≥ 1e9 is `moneyTooLarge` before the precision test (`1000000000.123` → reason 6, was 3): P1-7 says integer part ≥ 1e9 refuses with reason 6; no existing test pins the old order.
- Integer-stored money never goes through the tolerance (`parseIntegerMoney` untouched).

**Left unbuilt** — named so nobody assumes it exists:
- No configurable tolerance, no raised `realIntBound`, no snap warning — out of scope per spec.

**Traps** — things that look right and are not:
- Fixture `Amount` is bound as text into a REAL-affinity column; rendered text (`1.0e-05`, `999999999.9999999`) is SQLite's, not the literal — assert the quoted text from the red run, not the input.
- `Test_parseMoney_reads_every_real_just_below_the_bound_exactly` (`:68`) must stay on the digit-string fast path for ≤2 decimals; routing everything through `big.Rat` slows the 1e6 scan.
- `-0.0000001` must yield `0`, not a negated zero-cents path.
- Rat `SetString` accepts forms SQLite never renders (`1/3`, `0x`); call it only after the digit/exponent grammar guards.

## Phase report

Run B2 (steps 4-5) done, test-first.

Files: `internal/importer/money.go` — `parseRealMoney` now = sign strip + `realCents` (Inf, exponent/mantissa grammar, int-part >= 1e9 check, fast path) + `snappedCents` (`snapToCent` then post-snap bound `cents >= realIntBound*100` -> `moneyTooLarge`). Negative exponent goes straight to `snappedCents` with no int-part check (value is tiny); positive exponent stays `moneyTooLarge` before mantissa validation; malformed mantissa or empty/non-digit negative exponent -> `moneyNotANumber`. `internal/importer/money_internal_test.go` — flipped `5.5511151231257827e-17` to 0/OK; added rows `4.0e-07`, `2.0e-06`, `x.1e-05`, `1.0e-`, `999999999.9999999`, `-999999999.9999999`, `999999999.9999`.

Red first: 6 new rows plus acceptance 02 (`..._near_zero_residue_amount_as_zero_cents`) and 05 (`..._rounds_up_to_the_bound_as_too_large`) failed at their assertions; `2.0e-06` and `999999999.9999` green on arrival (refuse today), guarded by mutations. Narrow loop green; whole `internal/importer` package green; `golangci-lint run ./internal/importer/...` 0 issues.

Mutations (money.go, each restored, diff clean):
- drop post-snap bound (`case false:`): reds `real_just_under_the_bound_snaps_up_past_it`, `real_negative_just_under_the_bound_snaps_down_past_it`, acceptance `Test_import_refuses_a_residue_that_rounds_up_to_the_bound_as_too_large`.
- tolerance 1e-5: reds `real_negative_exponent_beyond_the_snap_tolerance` (2.0e-06), `real_beyond_the_snap_tolerance` (12.3400011), negative variant, `real_small_negative_exponent`, acceptance `..._beyond_the_snap_tolerance`, and `Test_import_refuses_an_exponent_form_amount_by_the_exponents_sign/a_small_negative_exponent_has_too_many_decimals`.

Next (V, steps 6-7): Sweep docs per plan (`money.go` doc comments — `parseMoney`, `parseRealMoney`, `realCents`, `snappedCents`; `offenders.go`, `splits.go`; phase1 spec/STATE/REVIEW-02), full verify with coverage gate, spec ticks, STATE.md, `status: done`. Do not redo: new helpers `realCents`/`snappedCents` are the structure; `exponent` var scoped inside the `if` to satisfy `wastedassign`.
