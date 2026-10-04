---
id: SCENARIO-03
status: open
---

# SCENARIO-03: Sync refuses an investment record quarry cannot read

Cadence: code-first (placeholders become offenders; no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_investments_test.go` `Test_run_sync_refuses_an_investment_record_quarry_cannot_read`
Narrow loop: `go test ./internal/importer/ && go test ./cmd/quarry/ -run '(?i)Investment|Unmappable'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 3 steps, importer

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_investments_test.go` `Test_run_sync_refuses_an_investment_record_quarry_cannot_read` — Outline rows (action 14, 1.23456789 shares, ratio 1:0, ratio 1:none, units with no position, nameless security) through `run sync`: exit 1, stdout empty, stderr = S4 frame + ruled reason, store bytes unchanged. Rows 1-2 are green on arrival (delivered by SCENARIO-02)

### Build
- [x] Step 2: `internal/importer/investments.go:19-23,196-208`, `reasons.go` — shares without a security becomes an offender (`classMissingValue`, dated) with `an investment transaction on <date> in "<account>" has shares but no security`; drop `errSharesWithoutSecurity`. Repoint `Test_import_fails_on_shares_without_a_security` (3 cases) at the reason
- [x] Step 3: `investments.go`, `reasons.go` — split with an unreadable side (NULL, zero, non-number, beyond scale, too large) becomes an offender (`classMissingValue`) with `a stock split on <date> in "<account>" of "<security name>" has a ratio quarry cannot read (<n>:<d>)`; side text is the raw column text, NULL renders `none`; no resolvable security drops the ` of "…"` clause. Drop `errUnreadableSplitRatio`. Repoint `Test_import_fails_on_a_split_with_an_unreadable_ratio` (6 cases) plus a no-security case and a security-named case
- [x] Step 4: `securities.go:36-58`, `importer.go:108`, `reasons.go` — ZNAME NULL or `""` (as accounts) adds an undated offender (`classMissingValue`, name `(source id N)`) with `a security (source id N) has no name`; the row stays mapped so its transactions raise no cascade refusal. Tests in `securities_test.go`: NULL, `""`, and a deleted nameless security ignored

## Handoff

Rulings S03 made (spec S.5 left them open):
- Ratio side text is the raw column text (`1`, `0`, `n/a`, `1.23456789`); non-number, beyond-scale and too-large sides all use the ratio copy, no separate copy
- A split with no resolvable security refuses with the ratio copy minus the ` of "<security>"` clause (variant of the ruled line; needs product-vision confirmation at the final pass)
- A security with no name is NULL or `""` (same as accounts)
- Offender classes: ratio, shares-without-security and security-no-name all `classMissingValue`

## Phase report

Run L done (steps 1-4 ticked; SCENARIO-03 NOT yet ticked in specification.md, STATE.md not yet rewritten, status still open — run V does those).

Red (before Build), `go test ./cmd/quarry/ -run Test_run_sync_refuses_an_investment_record`: 4 of 6 subtests failed at the stderr assertion — `a_split_ratio_1:0` and `..._NULL_denominator` got `quarry: cannot build the store in ~/Library/Application Support/quarry: has a split ratio quarry cannot read; run quarry sync --from <id>` (expected the S4 frame with the ruled reason); `non-zero_units_and_no_position` got the same shape with `has shares but no security`; `a_security_with_no_name` got `...: converting NULL to string is unsupported; ...`. Rows `an action code 14` and `1.23456789 shares` were green on arrival (SCENARIO-02). Now all 6 green.

Files: `cmd/quarry/run_investments_test.go` (+`os` import, new test at end); `internal/importer/investments.go` (placeholders and `errors` import removed; `buildInvestmentTransaction` returns `(txn, bool)` — error return dropped as always nil; new `resolveSecurity`, `ratioSideText`, `ratioSideNone`); `reasons.go` (`reasonInvestmentSharesWithoutSecurity`, `reasonSplitRatio`, `reasonSecurityNoName`); `securities.go` (`mapSecurities` takes `off`, ZNAME scanned as `sql.NullString`; nameless stays in the map); `importer.go:108`; tests `investments_test.go` (shares-without-security repointed; split ratio repointed, 8 cases, plus `Test_import_names_no_security_in_the_refusal_of_a_split_with_no_security`), `securities_test.go` (+3).

`go test ./internal/importer/` ok; `golangci-lint run ./internal/importer/... ./cmd/quarry/...` 0 issues. No full-suite, coverage, test-stats or spec tick yet (V). No mutations (plan: none).

V must not redo: reasons are ruled/derived as in Handoff; do not re-add an error return to `buildInvestmentTransaction`. V does: full Verify block with `<start>` 28efd71, tick SCENARIO-03 `cmd/quarry/run_investments_test.go` `Test_run_sync_refuses_an_investment_record_quarry_cannot_read` (note rows 1-2 delivered by SCENARIO-02), `spec-check.py`, STATE.md rewrite (drop the two S03 Left-unbuilt entries, the ZNAME trap, the S02 offender-class debt; keep the `<v>` text debt as unowned), `status: done`.
