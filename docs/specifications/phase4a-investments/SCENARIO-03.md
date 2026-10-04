---
id: SCENARIO-03
status: done
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
- Ratio side text is the raw column text (`1`, `0`, `n/a`, `1.23456789`); non-number, beyond-scale and too-large sides all use the ratio copy, no separate copy. `ZNUMERATOR`/`ZDENOMINATOR` are DECIMAL (NUMERIC affinity, `reference.sql`), so an integer-valued REAL is stored as an integer and prints `0`, never `0.0`
- A split with no resolvable security refuses with the ratio copy minus the ` of "<security>"` clause — contradicts S.5's "always named"; scoped product-vision copy ruling wanted before the final pass
- A security with no name is NULL or `""` (same as accounts); its quotes are not parsed (they would be refused as `a price of ""`), and the row stays mapped so its transactions raise no cascade refusal
- Offender classes: ratio, shares-without-security and security-no-name all `classMissingValue` (its comment now says so)

## Phase report

Run V done. Ruling update (spec S.5): a blob split side prints `blob`, never its bytes. Red first: `Test_import_shows_a_blob_split_side_as_blob` (REAL-style `UPDATE ... ZNUMERATOR = X'00FF41'`) failed at its assertion, actual `(\x00\xffA:12)`; `ratioSideText` (`internal/importer/investments.go`) now switches on `typ` null/blob; green.

Verify: `go build ./...` ok; full suite `go test -count=1 -coverpkg=./... ./...` rc=0; `uncovered-diff.py --profile ... 28efd71`: 0 uncovered added lines; `go test -race` importer and cmd/quarry ok; `golangci-lint run ./...` 0 issues. `test-stats.py --base 28efd71 --changed`: cmd/quarry 667 (+1), internal/importer 212 (+7), TOTAL 879 (+8), tempdir 667 (+3), disk 608 (+3). Spec ticked (spec-check OK), STATE.md rewritten, status done.
