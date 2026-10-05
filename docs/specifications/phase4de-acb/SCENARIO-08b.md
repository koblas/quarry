---
id: SCENARIO-08b
status: open
---

# SCENARIO-08b: ACB and gains per tax year

Cadence: code-first
Acceptance test: `cmd/quarry/run_acb_test.go` `Test_run_acb_prints_gains_per_tax_year_and_todays_acb_pooled_across_the_accounts`
Narrow loop: `go test ./internal/report/... -run '(?i)acb' && go test ./internal/cli/ ./cmd/quarry/ -run '(?i)acb|available_commands|no_store|older_quarry|interrupt|unknown_flag|home|config|skill_text'`
Mutation checks: `report.Today(now())` in the acb RunE → `Test_run_acb_counts_a_sale_dated_today_in_a_zone_ahead_of_utc`; unconditional `readConfig` in the acb RunE → `Test_acb_reads_the_config_once_even_with_the_currency_flag`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (report; `internal/cli` wiring and docs don't count)

Contract: `quarry acb [--currency <c>] [--json]`. Result goes to stdout, exit 0. Config warnings go to stderr first (`~` form) and lead `--json` `warnings` (absolute). Store refusals (no store, format 8, interrupt, HOME unset, bad config) exit 1. A positional argument or unknown flag exits 2.

Interim, until named later scenarios ship:
- `--year`/`--security` are NOT registered (S15/S16): an unknown flag exits 2, which is honest, where a registered no-op would silently print every security.
- `--currency` is registered with its ruled help and validated by `currencyFlag.args`. Its value is ignored and output is always CAD, labelled CAD (refusal: S17).
- Unclassified accounts are simply not in the pool (R-6: S17).

The acceptance test runs through `runWith` + `replaceStoreWithRates`, because the cli `fakeReportStore` has no history read.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_test.go` `Test_run_acb_prints_gains_per_tax_year_and_todays_acb_pooled_across_the_accounts`. Fixture:
  - Accounts: a CAD brokerage and a USD brokerage, both in `accounts.non-registered`; a registered account with its own buy/sell (must be absent from the output).
  - One CAD security bought in both pool accounts and sold from one, with a commission.
  - A USD buy + sell with a commission, plus `usdRate` rows.
  - Sells use negative stored shares. Clock as `holdingsClock()`.
  - Assert both text tables verbatim, exit 0, empty stderr.
- [x] Step 2: `internal/cli/acb.go` `newAcbCommand` registered at `internal/cli/root.go:28-41` (+ doc list `:7-10`). Use/Short only and a RunE returning nothing, so the test fails at its stdout assertion.

### Build
- [x] Step 3: report types and walk (`internal/report/acb.go:11-67`, `acb_walk.go:41-176`). **Additive only**: no existing field retyped (08a pins ~30 sites).
  - New fields:
    - `ACB.AsOf` (= `req.Today`).
    - `AccountID` + account name on `ACBSale` and `ACBEvent`, from `history.Accounts` (same read).
    - On `ACBEvent`: `Amount` (native cents, nullable for S11 adjustments), `Currency`, `Rate` (`money.Rate`, 0 when not USD), `CAD` = `toCAD(amount)` at the event rate, `Outlays *int64` (sale only), and a realized marker so JSON `gain` is null off a sale.
    - Zero-valued `PossibleSuperficialLoss`/`UnknownCost` on `ACBSale` and `Incomplete` on `ACBSecurity` (S12/13a set them).
  - Tests extend `acb_walk_test.go` / `acb_arms_test.go`: account ids/names per event and sale; USD event amount/rate/cad vs CAD event rate 0; outlays and marker only on a sell.
- [x] Step 4: `internal/report/document/acb.go` `NewACB(report.ACB, warnings) ACB` — full `--json` shape (spec lines 257-272):
  - `currency` "CAD"; `year` null; `as_of` date.
  - Money via `Money`; shares via `Shares` after Rat→millionths round-half-away; `acb_per_share` via `Rat.FloatString(4)`, null at 0 shares.
  - Ticker/currency nullable; per-year counts derived from sale flags; arrays `[]` never nil; events = full history.
  - `usd_cad` rendered from `money.Rate` millionths, trailing zeros trimmed to at least 4 decimals, null unless USD. `amount_currency` is null only when `amount` is.
  - Tests in `document/acb_test.go`: a hand-built `report.ACB` with every flag set and a sold-out security (per-share null); an empty ACB (`[]` arrays); a consolidation leaving non-terminating shares; read-back through `encoding/json` asserting key order and values.
- [x] Step 5: `internal/cli/render_acb.go` `renderACB` + `json_acb.go` `renderACBJSON`:
  - Year table: caption `Realized capital gains by tax year, in CAD`, columns `Year Sales Proceeds Outlays ACB Gain or loss`, years with a sale only, no total row.
  - Blank line, then `ACB on <as_of>, in CAD` with `Security Ticker Shares ACB ACB per share`: shares > 0 only; name via `escapeCell`; `formatShares`; thousands-grouped money; per-share 4 decimals.
  - No suffix column (S12/13a/14).
  - `render_acb_internal_test.go`: sold-out security omitted from text; security without a ticker; name needing escape; negative gain year.

- [x] Step 6: `internal/cli/acb.go` full command:
  - Ruled Use/Short/Long/Example verbatim (spec 209-234). Rewrap only the overlong Long line (spec :219) to the ~76-column width; words verbatim.
  - `--currency` via `currencyFlag.bind` with the ruled help; `Args: currency.args`.
  - RunE: `readConfig` **always** (never `currency.resolve`); `ACBRequest{classificationOf(cfg), report.Today(now())}`; `openReport`; `runtimeError` on fault; `emitReport` with `withConfigWarnings(cfg.WarningsAbsolute, nil)`.
  - Tests:
    - `internal/cli/acb_test.go`: help Long/Example/flag help pinned; `--json` read-back.
    - `cmd/quarry/run_acb_test.go`: `Test_run_acb_counts_a_sale_dated_today_in_a_zone_ahead_of_utc`; a `--json` run of the acceptance fixture; a config warning leading stderr and `warnings[0]`.
    - Add `InvestmentHistory` to `internal/cli/fakes_test.go:18`.
    - Join `acb` to `internal/cli/currency_test.go:203-250` (the accounts config-even-with-flag tests become a {accounts, acb} table).
  - **Not** in `currency_test.go:23`, `run_read_usage_test.go:81`, `run_usage_test.go:212` or `report_help_test.go:193`; S17 owns those.
  - All-commands rows to add:
    - `run_status_test.go:144-158`: root help, acb sorts first.
    - `run_read_refusals_test.go`: `:46-60` no store; `:98-106` `reporting.currency = "EUR"` (`config.Load` refuses it at `parse.go:314`); `:174` new `Test_run_acb_refuses_a_store_built_by_an_older_quarry`; `:228-240` interrupt.
    - `run_read_usage_test.go:45`: `quarry: acb takes no arguments`.
    - `run_usage_test.go:252-260`: unknown flag.
    - `run_config_test.go:237-247`: `readCommandArgs`.
    - `run_spend_refusals_test.go:~85-97`: HOME unset.
- [x] Step 7: docs.
  - `plugin/skills/quarry/SKILL.md:3`: description. Add `, or which investment accounts are registered` after `payee name variants)`; add `; adjusted cost base and realized capital gains per tax year (CAD)` after `its value on a day`; drop `, for gains or ACB beyond saying quarry does not cover them yet` (result: 1315 runes, under the 1536 cap).
  - `SKILL.md:74-75` §7: delete the ACB bullet; the Tax bullet becomes the ruled `**Tax:** quarry acb is a worksheet: …`.
  - `docs/initial-prd.md:256`: superficial loss marked in `acb`, not a finding (F1). The PRD `acb` row already exists at `:173`, so no step.
  - Re-pin `cmd/quarry/run_skill_text_test.go:165`, `:220-221`.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols; `doc.go`/root doc list mention acb.

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4de-acb`. Tick SCENARIO-08b with its acceptance test; rewrite STATE.md.

## Handoff

**Orchestrator rulings 2026-10-05:** deviations 1-8 accepted (`--year`/`--security` unregistered until 15/16; `--currency` registered, value ignored until 17; new fields not retyping 08a's; JSON `cad` = amount × its rate; shares format per holdings precedent). SKILL §7: keep the existing **Tax:** bullet (category totals, still true); REPLACE the ACB bullet (SKILL.md:74) with label `**Realized gains, ACB:**` + the ruled body verbatim: `quarry acb is a worksheet: say so, relay superficial-loss and incomplete warnings, never call a loss deductible or denied.` (no second Tax bullet). Long: rewrap the overlong line, words unchanged. Both go to the final product-vision pass.


**Binding decisions:**
- Every ACB caller passes `report.Today(now())`, never `now()`. `ACBRequest.Today` is compared with UTC-midnight dates, and a zone ahead of UTC would drop today's rows. S19's MCP caller included.
- `acb` reads config unconditionally (`readConfig`) and builds the classification only via `classificationOf(cfg)`. Adjustments (S11) and R-6 (S17) need it even with `--currency`.
- JSON event contract (S16's text columns read the same fields):
  - `cad` = the event's `amount` converted at its rate.
  - `amount`/`amount_currency` are null only for adjustments (S11).
  - `usd_cad` is null unless USD.
  - `outlays` and `gain` are non-null only on a sale. S11's ROC excess sets the same realized marker.
- 08a fields are never retyped. New report fields are additive. Flags (`PossibleSuperficialLoss`, `UnknownCost`, `Incomplete`) exist zero-valued; S12/13a set them, and the document already maps them.
- Shares follow the holdings precedent, not the spec examples (`410.000`, `"10"`): text uses `formatShares`, JSON uses `document.Shares`, after Rat→millionths round-half-away.

**Left unbuilt:**
- `--year`, `--security` flags and their output (S15, S16); empty-result text/warning 2 (S15/S18).
- `--currency` non-CAD refusal and R-6 refusal (S17). Year/position suffix column (S12/13a/14). `document.ACBWarnings` composer (first warning-owning scenario).
- SKILL §4 row, §6, trigger (S17); SKILL §9, `mcp --help`, MCP `acb` (S19).

**Traps:**
- `currency.resolve` skips `config.Load` when `--currency` is given. Using it in acb silently drops adjustments.
- `acb` must stay out of `currencyCommands` (`internal/cli/currency_test.go:23`): S17 refuses USD/native, and the accept-each-currency test would then fail.
- Split/consolidation `Held` can be a non-terminating `big.Rat`. Never `FloatString(6)` it unrounded or call `Num()/Denom()` on it directly.
- Deviations for the orchestrator to confirm before B2:
  - §7: the ruled Tax line replaces the old Tax bullet (face reading; no merged copy).
  - The Long :219 rewrap is layout only.

## Phase report

Run B2 done (steps 6-7). Acceptance `Test_run_acb_prints_gains_per_tax_year_and_todays_acb_pooled_across_the_accounts` is GREEN. Narrow loop green; `golangci-lint run ./...` 0 issues; full verification is run V.

Files:
- `internal/cli/acb.go`: ruled Use/Short/Long (:219 line rewrapped, words verbatim)/Example, `--currency` via `currencyFlag.bind(acbCurrencyHelp)` + `Args: currency.args`; RunE = `readConfig` always, `report.ACBRequest{classificationOf(cfg), report.Today(now())}`, `emitReport(nil warnings, renderACBJSON(withConfigWarnings(cfg.WarningsAbsolute, nil)), renderACB)`.
- `internal/cli/acb_test.go` (help Long/Example/--currency pinned, config classification reaches the walk, unlisted account out of pool, store-read and report-open faults, bad `--currency`); `internal/cli/fakes_test.go` `history` + `InvestmentHistory`; `internal/cli/currency_test.go` the four accounts-config tests are now `{accounts, acb}` tables (`configAlwaysReadCommands`).
- `cmd/quarry/run_acb_surface_test.go` (`--json` read-back of the acceptance fixture, config warning leading stderr and `warnings[0]`, zone-ahead-of-UTC today sale); all-commands rows in `run_read_refusals_test.go` (no store, EUR config, older store, interrupt), `run_read_usage_test.go`, `run_usage_test.go`, `run_config_test.go` (`readCommandArgs` acb row; acb skipped in the `--currency` ignores-malformed-config test, joined to accounts in the refuses-even-with-flag table), `run_spend_refusals_test.go` (HOME unset).
- Docs: `plugin/skills/quarry/SKILL.md` description (1315 runes) + section 7 ACB bullet (ruled body, label kept; heading still "Not covered yet"); `cmd/quarry/run_skill_text_test.go` re-pinned; `docs/initial-prd.md:256`.

Mutations (both red, restored byte-identical): `Today: report.Today(now())` -> `now()` reddened `Test_run_acb_counts_a_sale_dated_today_in_a_zone_ahead_of_utc`; readConfig skipped when `--currency` is given reddened the acb rows of `Test_config_always_read_commands_{read_the_config_once,refuse_an_unreadable_config,print_the_configs_warnings,name_the_configs_warnings_absolutely}...` and `Test_run_accounts_and_acb_refuse_a_malformed_config_even_when_given_a_currency/acb`.

Run V: sweep (lint already 0), full verification block, tick SCENARIO-08b in specification.md with the acceptance test, spec-check, STATE.md rewrite, `status: done`. Open for product-vision: SKILL section 7 heading vs the acb bullet; Example lists unregistered `--year`/`--security` (verbatim from the spec until S15/S16).
