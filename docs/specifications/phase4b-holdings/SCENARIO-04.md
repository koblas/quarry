---
id: SCENARIO-04
status: open
---

# SCENARIO-04: Native currency lists each currency's own total

Cadence: code-first — read-only report and renderer, nothing on the mandatory set
Acceptance test: `cmd/quarry/run_holdings_test.go` `Test_run_holdings_native_lists_each_currencys_own_total`
Narrow loop: `go test ./internal/report/ ./internal/cli/ ./internal/report/document/ ./cmd/quarry/ -run 'Holdings|holdings'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 3 steps, report+cli

Spec: S.2 native rules, R1, S.5 `totals`; H-3. No new copy lines (no help, warning or refusal): `Total` rows and the caption already ruled; NULL/EUR warnings belong to S07.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_test.go` `Test_run_holdings_native_lists_each_currencys_own_total` — the S03 CAD + USD + closed-account fixture (extracted to a shared seed helper) run with `--currency native`: no In column, caption without `, amounts in`, `Total` rows CAD then USD with Currency and Value cells, empty stderr, exit 0

### Build
- [x] Step 2: `internal/report/holdings.go:56-70` `total()` — native sums each row's own `Value` per non-NULL stored currency, ordered CAD, USD, then alphabetical; NULL currency or nil `Value` never contributes; CAD/USD modes unchanged. Tests `internal/report/holdings_test.go`: order from input EUR, USD, AUD, CAD (CAD, USD, AUD, EUR); one total per currency sums both rows; NULL-currency row not totalled; unpriced row alone gives no total; zero value alone gives `0`
- [x] Step 3: `internal/cli/render_holdings.go:41-43` native arm — one `Total` row per `h.Totals` entry with Currency and Value cells filled (`holdingsNativeTotalRow`). Tests `render_holdings_internal_test.go` (two currencies, in order; no totals → no Total row); JSON `totals` one entry per currency in that order, read back in `internal/report/document/holdings_test.go`

### Sweep
- [ ] Step 4: `go build ./... && golangci-lint run ./...` at `0 issues`; doc comments

### Verify
- [ ] Step 5: full verification, `spec-check.py`, tick SCENARIO-04, rewrite STATE.md

## Handoff

- Native totals live in `report.Holdings.total()` only; renderers and `document` format `Totals`. S08 appends the unconverted per-currency totals to the same slice in converted modes.

## Phase report

Run L done (steps 1-3); `V` (steps 4-5) left.

- Red: `Test_run_holdings_native_lists_each_currencys_own_total` failed at its assertion (`cmd/quarry/run_holdings_test.go`): actual stdout ended after the Old RRSP row, the two `Total` rows were missing. Green after steps 2-3.
- `cmd/quarry/run_holdings_test.go`: S03 fixture extracted to `seedHoldingsStore`, `holdingsClock`, `holdingsNativeLine` helpers; S03 acceptance test unchanged in behaviour.
- `internal/report/holdings.go`: `total()` branches to `nativeTotals()` (per non-NULL stored currency, `nativeRank` CAD 0 / USD 1 / rest 2 then `strings.Compare`); nil `Currency` or nil `Value` never contributes. Converted modes unchanged. New imports `cmp`, `slices`, `strings`.
- `internal/cli/render_holdings.go`: native arm appends `holdingsNativeTotalRow` per `h.Totals` entry (Currency cell = width-2, Value cell = width-1).
- Tests: `internal/report/holdings_test.go` (+4: order, per-currency sum, NULL excluded, unpriced/zero), `internal/cli/render_holdings_internal_test.go` (+1), `internal/cli/holdings_test.go` (+1, native `--json` totals order and null `converted_value`). No change needed in `document` (it formats `Totals` in order); its existing tests cover that.
- Not done: lint, doc.go/STATE updates, full suite, spec tick (V). STATE.md still says "none in native" for Totals (H-3 entry) and lists native Total rows as left unbuilt: V rewrites both.
