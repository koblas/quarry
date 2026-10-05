---
id: SCENARIO-07
status: done
---

# SCENARIO-07: ACB adjustments are read from config

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `internal/config/adjustment_test.go` `Test_load_reads_acb_adjustments_as_exact_cents`
Narrow loop: `go test ./internal/config/ ./internal/platform/money/ -run 'Adjust|ParseCents|Load'` then `go test ./cmd/quarry -run 'Test_run_.*adjustment'`
Mutation checks: amounts read from raw token text → `Test_money_ParseCents_is_exact_where_float_is_not` (swap to `strconv.ParseFloat`*100 must redden on 1.15 / 0.29); two-decimal bound → `ParseCents` 3-decimal row and `Test_load_refuses_an_adjustment_amount_that_is_not_above_zero_with_two_decimals`; refusal precedence (presence before type) → `Test_load_reports_a_missing_key_before_a_wrong_type_in_the_same_item`
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 Build batches (B1 ≤3, B2 1), 1 feature package (`internal/config`) + `platform/money`; matches sizing row

## Existing surface (survey, no Glob needed)
- `config.go:15-39` `Config` (add field); `config.go:42-47` `Load` doc lists the refused settings (extend, trim to budget — open debt).
- `parse.go:79-120` `(file).parse` runs checks in fixed order, doc comment :77-78 names it; `parse.go:~393-407` `knownKeys`; `parse.go:~183-196` `entries()` already emits `ArrayTable` entries with full key path, KeyValues under a header carry the header's path (`[acb adjustment security]`), so items group by splitting at each `[acb adjustment]` ArrayTable entry; `parse.go:~268-274` `(document).written`; `parse.go:~312-325` `got` (ArrayTable → `gotTableList`, Table → `gotTable`).
- `refusal.go:~46` `(file).badValue` always appends `, got <v>` — the three `needs …` lines have no `got`; they need a sibling that takes the whole reason (via `(file).refuse`, which already yields ~ and absolute forms and `fixIt`).
- `platform/money/money.go` has no decimal parser (only `ParseCurrency`, `Convert`). `duckstore/shares.go:242` `parseDecimal` is `big.Rat`, store-internal, not reusable. Dates elsewhere are `time.Time` midnight UTC (`snapshot/import.go:103`).
- Config is built only in `config.Load`; no caller constructs a positional `Config`, so a new field is additive. Command-level refusal pins to copy: `cmd/quarry/run_config_test.go:145` (`sync`, case table) and `:287-338` (`readCommandArgs()` loop, `Test_run_read_commands_refuse_a_masked_account_list`).

## Contract
Refusal = `quarry: <~path>: <line>; fix the file and run the command again`, exit 1, nothing on stdout, for `sync` and every read command. Lines verbatim from `specification.md` `#### Adjustments`; `<n>` 1-based file order, first bad item wins. Check order inside one item (ruled order): needs security, needs date, needs an amount, then security type, date type, amount value (return-of-capital before reinvested-distribution). `got <v>` is the value's text as written (`d.got`-style, one line). Valid item = `security` non-empty string (quotes may be literal `'`), `date` TOML local date, amount a bare number matching `^[0-9]+(\.[0-9]{1,2})?$` and > 0. Quoted `"12.34"`, `1e2`, `1_000.5`, `-5`, `0`, `12.345`, `12.340` all refused with the amount line. Unknown subkeys, `acb.<other>` → existing unknown-key warning (both `Warnings` and `WarningsAbsolute`). Both amounts in one item: ALLOWED (reading: line 3 says "needs return-of-capital or reinvested-distribution", not "exactly one"; no new copy). `Config.Adjustments` carries both.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/config/adjustment_test.go` (new) `Test_load_reads_acb_adjustments_as_exact_cents` — file with the spec's two `[[acb.adjustment]]` examples plus one item with both amounts; asserts `cfg.Adjustments` (security, date, cents) in file order, empty `Warnings`
- [x] Step 2: `config.go:15-39` `Config.Adjustments []Adjustment` + `Adjustment{Security string; Date time.Time; ReturnOfCapital, ReinvestedDistribution int64}` (cents, 0 = not given) — types only; fails at its assertion

### Build
- [x] Step 3: `platform/money/money.go` `ParseCents(text string) (int64, bool)` + `money_test.go` — unsigned decimal text only. Rows: `12`, `12.3`, `12.34`, `0`/`0.00` (parses to 0; caller owns >0), 3 decimals, `12.340`, sign, `.5`, `1_0`, `1e2`, hex, empty, int64 edge (`92233720368547758.07` ok, `.08` refused), exact `1.15`/`0.29`
- [x] Step 4: new `internal/config/adjustment.go` `(document).adjustments()` called from `parse.go:~104` after `notInBoth` (update `parse()` doc order, `Load` doc); `refusal.go` `(file).refuseText`-style helper for no-`got` lines — item grouping from `d.entries`, line 1 (`acb` or `acb.adjustment` not a list of tables written as `[[ ]]`: inline array, `[acb.adjustment]` table, scalar, `acb = 5`, `[[acb]]`) and the three `needs` lines; tests: each line, item number 2 when item 1 is good, missing amount with both absent, empty `[[acb.adjustment]]` item, valid file with no adjustments → `nil`
- [x] Step 5: `adjustment.go` type/value refusals — security not a string (`41`, `""`), date not a local date (quoted string, `2024-12-31T10:00:00Z`, integer; impossible date is TOML syntax error, assert whichever refusal `Load` gives), amount line per amount key (both keys, key named in line); tests: amount bounds `0`, `0.00`, `-5`, `12.345`, `12.34` control, quoted; `Test_load_reports_a_missing_key_before_a_wrong_type_in_the_same_item`; absolute-path form via `ProblemAbsolute` for one row
- [x] Step 6: `parse.go:~393-407` `knownKeys` += `{acb}`, `{acb adjustment}`, four `{acb adjustment <key>}`; tests: no warning for a full valid file; `memo` inside an item warns `…: unknown key acb.adjustment.memo; quarry ignores it`; `acb.other` warns; letter-case variant `Date` warns. Command pins in `cmd/quarry/run_config_test.go`: new case in the `sync` ruled-copy table (`:145`) and a read-commands loop test (like `:300`) with a bad item, asserting exact stderr, empty stdout, exit 1

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Adjustment`, `ParseCents`; trim `Load` doc (open debt: `config.go:42-47`) while touching it

### Verify
- [x] Step 8: full verification + `.claude/scripts/spec-check.py phase4de-acb` → tick SCENARIO-07 with its acceptance test; rewrite STATE.md

## Handoff

**Orchestrator 2026-10-05:** decisions 1, 2, 4 accepted (both amounts in one item allowed — S11 applies both; check order as ruled-line order; empty security refused with the security line). Decision 3 replaced: `acb` set to a non-table uses the existing `lookup` "must be a table, such as …" template (the 4d `accounts = 5` precedent) with example `[[acb.adjustment]]` — not line 1.


**Binding decisions:**
- `config.Config.Adjustments []Adjustment{Security string, Date time.Time (UTC midnight), ReturnOfCapital, ReinvestedDistribution int64 cents}`, file order, nil when none — S08b/S11 read it from `cfg`; no other config type
- Amounts are parsed from raw token text by `money.ParseCents`, never float64; the `>0` rule lives in config, not in `ParseCents`
- One item may carry both amounts (allowed, no new copy); S11 must apply both — flag to S11's architect, and to product-vision only if a ruling on order within a day is wanted
- Item checks run presence-first then type, per the ruled line order; first bad item wins; a bad item refuses every command

**Left unbuilt:**
- The three adjustment warnings (unknown security, not held on date, duplicate items) — S11 (needs the store; `config` stays store-free)
- Any consumer of `cfg.Adjustments` (`report`, `acb`) — S08b/S11

**Traps:**
- Read commands skip `config.Load` when `--currency` is given (STATE.md); `acb` (S08b) must load config regardless or adjustments silently vanish
- `badValue` always appends `, got <v>`; the `needs …` lines must not use it
- `acb` set to a non-table is not covered by a ruled line: this plan refuses it with line 1 (`got` = its text) rather than ignoring it silently; `{acb}` being a known key means the unknown-key path will not catch it — copy not separately ruled, mention at the final product-vision pass
- Quoted `'sec-41'` (literal string) is a string and accepted; an impossible calendar date may fail as TOML syntax (`cannot read …: line n`) not as ruled line 6 — pinned as whatever `Load` returns; confirm `toml.LocalDate` is what go-toml v2 yields into `map[string]any` before relying on it

## Phase report

All runs done (A, B1, B2, V); acceptance `Test_load_reads_acb_adjustments_as_exact_cents` GREEN, full suite rc=0, uncovered-diff 0, lint 0 issues. V trimmed the `Load` and `setting` docs; `Adjustment`/`ParseCents` docs already existed.
- B2: `adjustment_test.go` `Test_load_warns_of_an_unknown_key_beside_acb_adjustments_in_both_warning_lists` (memo inside an item, `acb.other`, `Date`; both warning lists), green on arrival because B1 owned `knownKeys`. Mutation: adding those three keys to `knownKeys` reddened exactly those three subtests (restored).
- B2: `cmd/quarry/run_config_test.go` pins: `sync` ruled-copy row "an adjustment without a date" and `Test_run_read_commands_refuse_a_bad_acb_adjustment` (item 2 amount 12.345, every `readCommandArgs`, exact stderr, empty stdout, exit 1). Green on arrival (production is B1's); copy matches `specification.md` lines 316-321.
- V must do: `Load` doc budget debt (`config.go:42-47`), `setting` doc, `Adjustment`/`ParseCents` docs, full verification, spec tick, STATE.md, `status: done`.
- Run B1 notes follow.
- `internal/platform/money/money.go:~103-130` `ParseCents` (+2 table tests in `money_test.go`). Rune loop, no regexp/float; `>0` stays in config.
- `internal/config/adjustment.go` (new): `(document).adjustments()`, `itemTexts()` (groups `d.entries` by `[[acb.adjustment]]` header; per-item raw text), `adjustment()`, `amount()`, `textOf()`. Called in `parse.go` after `notInBoth`; `parse()` and `Load` docs updated. `refusal.go` `(file).badItem(reason)` = no-`got` refusal.
- `acb` non-table and `[[acb]]` go through the existing `lookup` with `adjustmentSetting` (example `[[acb.adjustment]]`), per the orchestrator ruling; line 1 is only for `acb.adjustment` itself (inline array, `[acb.adjustment]`, scalar, dotted keys).
- Why run A saw no refusal: `gotTableList` is reached only via `d.got` for an already-refused known setting; nothing looked at `acb`, so `[[acb.adjustment]]` was just an unknown-key warning. Plan misread, not a bug.
- `knownKeys` (6 `acb` entries, `parse.go`) was added in B1, not step 6, because the acceptance test asserts empty `Warnings`.
- Tests: `adjustment_refusal_test.go` (new): tables for line 1, acb non-table, needs-lines, security/date type, amount; precedence, item 2, first-bad-item, absolute form, nil when none; an impossible date is a TOML syntax error, `cannot read ...: line 3:` (ErrorContains).
- Mutations (restored, diff clean): float parse in ParseCents reds `…ParseCents_is_exact_where_float_is_not/1.15` (expected 115, actual 114) and `/0.29`; dropping the 3-decimal bound reds ParseCents `three_decimals` rows and `Test_load_refuses_an_adjustment_amount_that_is_not_above_zero_with_two_decimals/three_decimals`; `case !hasDate:` to `!hasDate && false` reds `Test_load_reports_a_missing_key_before_a_wrong_type_in_the_same_item`.
- Narrow loop green; `golangci-lint run ./...` 0 issues.
- Trap: narrow-loop `-run 'Adjust'` is case sensitive and matches no test (names say `adjustment`); use `(?i)adjust`.
