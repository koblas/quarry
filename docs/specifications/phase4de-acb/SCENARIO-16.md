---
id: SCENARIO-16
status: open
---

# SCENARIO-16: One security's history

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_acb_security_test.go` `Test_run_acb_security_prints_every_event_of_the_named_security_with_shares_held_acb_and_gain`
Narrow loop: `go test ./internal/report/... ./internal/cli/ -run 'ACB|Acb|acb' && go test ./cmd/quarry/ -run 'Test_run_acb'`
Mutation checks: registered-only list set on the UNCUT report in `(*Server).ACB` (feed `ACBWarnings` the cut instead) → `Test_run_acb_security_warns_a_security_held_only_in_registered_accounts`; `Years` filtered and re-summed in `(ACB).Cut` (keep unselected sales) → `Test_acb_cut_keeps_only_the_named_securities_sales_and_totals`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`report` + its `document` sub-package; cli/cmd wiring)

## Pending rulings (orchestrator gets these ruled BEFORE run A; plan is built on the recommended default)
Copy, for `product-vision` (mid-feature copy ruling):
- P1 registered-only warning slot — default: right after slot 2 (both explain an empty answer), one line per security, walk order
- P2 caption when ticker is nil — default: `ACB history of "<name>", in CAD` (drop ` (<ticker>)`)
- P3 text when every named security is registered-only — default: caption + header, no rows, per such security (stdout never blank, S15); JSON `securities` stays `[]` as ruled
- P4 history suffix column (unheaded, last) — default: `possible superficial loss` (sell whose sale is marked in `Years`), then `unknown cost` (`ACBEvent.UnknownCost`, acquisitions and removals alike), ", "-joined. JSON event key `unknown_cost`: default NOT added (event key order ruled without it); if ruled in, append after `gain`
- P5 Rate cell — default: the JSON `usd_cad` form (`document.formatRate`, exported as `document.Rate`); blank for CAD and on an unvalued event
- P6 adjustment row cells — default: Account, Shares, Amount, Rate blank; CAD/ACB/Shares held filled; Gain only on a ROC excess
- P7 `--year` with `--security` — default: compose, no refusal: selection then year cut; text = the `--year` sales table restricted to the named (renderer branches on `Year` first)
Defaults, for the orchestrator:
- D1 matching — copy `pickAccount` (`internal/report/accounts.go:127-155`): `""` matches nothing; exact id first and alone; else every security whose ticker OR name `strings.EqualFold`s it. DROPPED arm: an ambiguous match is not refused — a shared ticker returns every id (warning 7's case)
- D2 a security matched by several selectors appears once; blocks in walk order (name case-folded, then id), never argv order
- D3 several selectors match nothing → refuse naming the FIRST in argv order; refusal is `RefusalError{Kind: RefusalUnknownSecurity, Arg}`, `%q`, exit 1
- D4 matched but in neither pool nor any registered-account transaction (<= Today) → same refusal as no match (the registered-only line would be false)
- D5 warnings 1, 3-9 under `--security` come from the UNCUT report (S15 rule extended); JSON `years` = only the named securities' sales, totals re-summed, ROC excess only from named with `NoRate == nil`; `year` null without `--year`
- D6 `--json` prints the JSON document only (as every command)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_security_test.go` (new) `Test_run_acb_security_prints_every_event_of_the_named_security_with_shares_held_acb_and_gain` — buys, a sell and a config `[[acb.adjustment]]` on one security plus a second security that must not print; `acb --security <ticker>`; full stdout block (caption, header, every row incl. the adjustment), stderr empty, exit 0
- [x] Step 2: `internal/cli/acb.go:21-92` `--security` via `StringArrayVar` + `acbSecurityHelp` const; `internal/report/acb.go:16-21` `ACBRequest.Securities`; stub so it compiles and fails at the stdout assertion

### Build
- [x] Step 3: `internal/report/acb.go:16-50,207-217` + new `acb_select.go` — resolve selectors in `(*Server).ACB` against `history.Securities` (the one read; registered-only securities never reach `ACB.Securities`, `acb_walk.go:72-80`); uncut `ACB` gains the selected ids, a "selection given" flag and `RegisteredOnly []store.Security`; `refusal.go:18-24` `RefusalUnknownSecurity` + `unknownSecurityRefusal` beside `:86-92`, `Arg` doc `:36`. Tests `internal/report/acb_select_test.go`: one row per D1 arm (id, ticker case-folded, name case-folded, shared ticker → both, `""`, id that is also another's name → id alone), D2 dedupe, D3 first-unmatched among matched, D4 adjustment-only/after-today security, registered-only row (control: same security also in pool → selected, not registered-only). Fault tests: n/a, selection is pure over the existing read (store fault already pinned by `Test_acb_returns_a_failed_store_read`)
- [x] Step 4: `internal/report/acb_unknown_cost_test.go:194-217` — remove_shares-outside-span control row (removal with no prior no-cost add → mark false), lands before the renderer exposes the mark (STATE open debt)
- [ ] Step 5: `internal/report/acb_year.go:10-28` new `(ACB).Cut()` = selection then `InYear` (InYear stays exported); filters `Securities` and `Years`, re-sums totals per D5 (mirror `acb_walk.go:82-85` for `NoRate`). `acb_year_test.go`: `Test_acb_cut_keeps_only_the_named_securities_sales_and_totals`, no-selection = `InYear` unchanged, selection + year (P7), named no-rate security leaves its ROC excess out, a year left with no named sale disappears
- [ ] Step 6: document + cli — `document/acb_warnings.go:18-28` registered-only slot per P1 (`acb_warnings_slots_test.go` order row, `acb_warnings_test.go` copy pin, acbPooled helper per Trap); export `formatRate` (`document/acb.go:206-214`). `internal/cli/render_acb.go:19-25` branch on the selection flag → new `renderACBHistory` (caption P2, 10 columns + suffix P4, Amount `-1,234.56 USD`, Rate P5, CAD and Gain blank when `Unvalued` (S14 ruling), Gain blank unless `Realized`, `escapeCell` on name/account, blank line between blocks, P3); `cli/acb.go:79-86` wire `acb.Cut()`. Tests: `internal/cli/acb_test.go:102-109` flag-help pin `--security name +show the full history ...`; `render_acb_internal_test.go` cell arms (CAD vs USD rate, unvalued, adjustment P6, nil ticker, marks order); `run_acb_security_test.go` cmd matrix: `Test_run_acb_security_warns_a_security_held_only_in_registered_accounts` (`acbRows` `sec-maple`, `run_acb_test.go:36-62`; text + `--json`), refusal `quarry: no security named "XYZ"; run quarry acb to list the securities it covers` exit 1 (unmatched, `""`, one unmatched among matched), repeated `--security` two blocks walk order, `--json` `securities` = named and `years` cut (read back with `encoding/json`), `--year`+`--security`, shared-ticker pair, id selector. Folded 4b debt: new cmd fixture with a no-cost reinvest; run the warning's literal `quarry acb --security <id>` and assert the reinvest rows carry `unknown cost`; removal row inside span carries `unknown cost` in text

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (expect the `exhaustive` arm at `internal/mcp/result.go:75-89`: route `RefusalUnknownSecurity` to `refusal.Error()` for now); doc comments on `Cut`, `ACBRequest.Securities`, new fields, `renderACBHistory`

### Verify
- [ ] Step 8: full verification + `.claude/scripts/spec-check.py phase4de-acb` → tick SCENARIO-16 with its acceptance test; STATE.md rewrite (Cut binding, removed S16 Left-unbuilt rows, P-rulings recorded)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Selection is resolved in `(*Server).ACB` from the one `InvestmentHistory` read; results live on the UNCUT `ACB` — `ACBWarnings` reads the uncut report, a cut-only field silently drops the registered-only line
- `(ACB).Cut()` is the ONE cut (selection, then year); replaces STATE's "`InYear()` is the ONE cut". S19's MCP `security`/`year` params set `ACBRequest` and render `Cut()`, warnings from the uncut report
- The cut filters `Years` with `Securities` — `document.NewACB` (`document/acb.go:99-102`) and `renderACBSales` (`render_acb.go:31-34`) name sales from `a.Securities` only; an unselected sale renders nameless
- The renderer branches on "selection given", never on emptiness (S15 rule)
- Unknown `--security` refusal is OWNED here (as S15 owned `--year`'s); S17 drops that row and rules whether its R-6 refusal precedes this one (both follow the one read)

**Left unbuilt** — named so nobody assumes it exists:
- MCP wording of `RefusalUnknownSecurity` (`internal/mcp/result.go` `refusalLine`) and the MCP `security` param — S19
- JSON event `unknown_cost` key — only if P4 rules it in; otherwise final product-vision pass
- `acb` rows in the `currencyCommands` tables — S17 (unchanged)

**Traps** — things that look right and are not:
- `StringSliceVar` splits on commas; a security name can contain one — use `StringArrayVar`
- Ticker matching here is case-INSENSITIVE (user input); grouping for warning 7/superficial stays exact case-sensitive (`groupingTicker`) — do not share the helper
- `walkACB` skips a security with no pool rows (`acb_walk.go:74-80`): registered-only detection must read `history.Transactions` + `Classification.Of == true`, not `ACB.Securities`
- A no-cost reinvest's Amount and CAD are 0: without the `unknown cost` suffix, warning 4b's "lists them" is unprovable

**Orchestrator: product-vision ruling 2026-10-05 recorded in spec after "Text `--security`" — supersedes this plan: P4 JSON event key `unknown_cost` IS added (last, after gain; re-pin every event-byte pin in internal/report/document/acb_test.go); refusal line replaced: `quarry: acb covers no security named "XYZ"; quarry acb --json lists every security it covers` (exit 1); P3 widened: every named security gets a block in walk order, registered-only = caption+header, incl. mixed selections; slot 2b for the registered-only warning; P1, P2, P5-P7, D1-D6 confirmed.**

## Phase report

Run B1 (steps 3-4) done. Run A's red acceptance test is still red (flag accepted, nothing renders the selection) -- B2 turns it green.
- `internal/report/acb_select.go` (new): `(ACB).withSelection(history, req)` returns the uncut ACB with `Selected`, `SelectedIDs` (walk order: name case-folded, then id; pooled or not) and `RegisteredOnly []store.Security`; called from `(*Server).ACB` (`acb.go:~222`) only when `len(req.Securities) > 0`. D1 in `matchSecurities` (id alone, else ticker/name `EqualFold`, `""` none); D4 via `registeredHoldings` (any action, registered account, <= Today). Per selector: matches filtered to pooled-or-registered; none left -> `unknownSecurityRefusal(selector)` (first in argv order, no partial ACB).
- `ACB.Securities`/`Years` are NOT cut by selection: B2's `(ACB).Cut()` filters `Securities` by `SelectedIDs` and `Years` by sale `SecurityID`; the renderer walks `SelectedIDs`, taking each from `Securities` or `RegisteredOnly`.
- `internal/report/refusal.go` `RefusalUnknownSecurity` + `unknownSecurityRefusal` (copy per ruling: `acb covers no security named %q; quarry acb --json lists every security it covers`); `internal/mcp/result.go:~85` routes it to `refusal.Error()` (S19 words it). `acb_walk.go` `nonRegisteredAccounts` became `accountsClassified(accounts, c, registered bool)`.
- Tests: `internal/report/acb_select_test.go` (9 tests, shared `selectHistory` fixture: pooled sec-1/2/6/7/8, registered-only sec-3, after-today sec-5, unclassified-only sec-9, no-tx sec-4); `acb_unknown_cost_test.go` new `Test_acb_leaves_a_removal_before_any_shares_were_added_with_no_cost_unmarked` (green on arrival: the walk already marks only inside the span; it is a control for B2's removal mark).
- Green: `go build`, `golangci-lint run ./...` 0 issues, report/cli/mcp narrow tests. Red (expected): `Test_run_acb_security_prints_every_event_...`.
- B2 must not redo: selection/refusal. Still unbuilt: `Cut`, slot 2b, `document.Rate`, `renderACBHistory`, `acb.Cut()` wiring, JSON `unknown_cost` (re-pin `document/acb_test.go` event bytes), flag-help pin, cmd matrix.
