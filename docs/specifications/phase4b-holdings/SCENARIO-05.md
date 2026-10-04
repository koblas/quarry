---
id: SCENARIO-05
status: open
---

# SCENARIO-05: Holdings on a past date (folds SCENARIO-11, refuses a date it cannot use)

Cadence: code-first (no mandatory test-first item: parsing and a refusal, no file write, atomicity or bug fix)
Acceptance test: `cmd/quarry/run_holdings_as_of_test.go` `Test_run_holdings_as_of_a_year_lists_the_shares_after_a_split_before_it`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_holdings_as_of_test.go` `Test_run_holdings_refuses_a_date_it_cannot_use`
Narrow loop: `go test ./internal/report/ -run 'AsOf' && go test ./internal/cli/ -run 'Holdings' && go test ./cmd/quarry/ -run 'Holdings'`
Mutation checks: today clamp in `ParseAsOf` (drop it, a current month is refused) → `Test_parse_as_of_resolves_the_current_year_and_month_to_today`; future guard (`> today` to `>= today`, and delete it) → `Test_parse_as_of_refuses_a_day_after_today_and_accepts_today`
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 2 batches, 1 feature package (`report`); `cli` and `cmd/quarry` do not count

## Implementation Plan

Surface read (no new port, so no port survey): the `report.Store.Holdings` port already takes `HoldingsParams{AsOf}`
(`report/store.go:26-27`, `report/holdings.go:44`) and the caption already prints `h.AsOf` (`cli/render_holdings.go:61-62`).
Only the CLI source of `AsOf` changes (`cli/holdings.go:47` `report.Today(now())`). Found by `go doc` + anchored grep; the
`--as-of` string appears nowhere else in `.go` files except the Long/Example pins in `cli/holdings_test.go:79,87,105`.

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_holdings_as_of_test.go` (new) `Test_run_holdings_as_of_a_year_lists_the_shares_after_a_split_before_it` — own seeder `seedSplitHoldingsStore` (copy the `replaceStoreWithRates` shape of `run_holdings_test.go:33-65`): one CAD Brokerage holding, buy 100 on 2025-06-02, `ActionSplit` (`SplitNewShares`/`SplitOldShares` 2:1) on 2025-09-15, buy 50 on 2026-02-10; prices 10.00 on 2025-06-02, 12.00 on 2025-12-30, 15.00 on 2026-01-05 (one after the as-of day); one rate row. Clock `holdingsClock()` (2026-03-12). `--as-of 2025` → caption `Holdings on 2025-12-31 …`, 200 shares, price 12.00, `Priced on` 2025-12-30. Red: unknown flag, exit 2
- [ ] Step 2: same file `Test_run_holdings_refuses_a_date_it_cannot_use` — table over `seedHoldingsStore`: `2024-13` and `2099-01-01`, stderr = the two S.4 lines verbatim (`quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`; `quarry: --as-of 2099-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of`), exit 2, stdout empty. Red at the stderr assertion (cobra unknown-flag line). No stubs needed, both compile today

### Build
- [ ] Step 3 (B1): `internal/report/asof.go` (new) + `asof_test.go` (new) — `ParseAsOf(value string, now time.Time) (time.Time, error)` and `AsOfError{Kind, Value}` (`AsOfNotADate`, `AsOfAfterToday`; `Error()` is the CLI wording with `--as-of`, no `quarry: ` prefix; a distinct type so `mcp/window.go:13-40` needs no new `WindowErrorKind` case; S13 words it for `as_of`). Reuse the layouts of `dateForms` and the first/last-day step of `parseDateBound` (`report/window.go:50-58,139-146`) by extracting a shared helper, not a copy. Rule: a period whose first day is after today is refused; otherwise the result is `min(last day of the period, Today(now))` (a period containing today clamps to today). `AsOfAfterToday.Value` is the resolved date (`2099` → `2099-12-31`, `2026-11` → `2026-11-30`); `AsOfNotADate.Value` is the value as typed. Tests (table, `windowNow` and `day` from `window_test.go`):
  - forms: `2025` → 2025-12-31; `2024-02` → 2024-02-29; `2023-02` → 2023-02-28; `2025-06-15`; `2024-02-29` ok, `2025-02-29` refused (as typed)
  - today arms, each its own row with its control: today itself ok; yesterday ok; tomorrow refused naming tomorrow; current year `2026` → today, no refusal; current month `2026-09` → today; next month `2026-10` refused naming 2026-10-31; next year `2027` refused naming 2027-12-31; a day inside the current month but after today (`2026-09-30`) refused naming itself
  - not-a-date rows, each quoted as typed: `2024-13`, `2025-00`, empty string, ` 2025` and `2025 ` (whitespace is not trimmed), `2025-1`, `last spring`, `2025-06-15T00:00:00Z`
  - zone: `now` late evening in `utcMinus5`-style zone whose UTC day is tomorrow resolves through `Today(now)` (the clock's own day), as `Test_today_is_the_calendar_day_in_the_instants_own_zone`
- [ ] Step 4 (B2): `internal/cli/holdings.go:13,35-47,60` `newHoldingsCommand` — `var asOf string`, `cmd.Flags().StringVar(&asOf, "as-of", "", holdingsAsOfFlagHelp)`; flag help verbatim S.1 `value holdings on `date` (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)`. In `RunE`, **before** `currency.resolve` and before opening the store: not given (`!cmd.Flags().Changed("as-of")`) → `report.Today(now())`; given (including `""`) → `report.ParseAsOf`, error → `UsageError{msg: err.Error()}` (exit 2 via the existing mapping, as `cli/window.go:49-52`). Tests in `cli/holdings_test.go` (const `holdingsAsOfHelp` beside `holdingsCurrencyHelp:23`): `--help` shows `--as-of date +<help>$` by regexp (as `:122`); fake-store read gets `HoldingsParams{AsOf}` 2025-12-31 for `--as-of 2025` (control: default still reads the clock day, `:135-143`); `--as-of 2025 --json` document `as_of` is `2025-12-31`; `--as-of 2025 --currency native` caption `Holdings on 2025-12-31 in all accounts; cash not included`; a bad value returns `UsageError` text and the fake is never read (assert `got` zero); bad `--as-of` plus bad `--currency` → the as-of line (as-of parsed first); `--as-of ""` refused as `--as-of "" is not a date …`
- [ ] Step 5 (B2): `cmd/quarry/run_holdings_as_of_test.go` — `Test_run_holdings_as_of_forms_pick_the_shares_of_their_day` (split-day boundary on `seedSplitHoldingsStore`: `2025-09-14` 100 sh price 10.00; `2025-09-15` 200 sh; `2025-09` → caption 2025-09-30, 200 sh, 10.00; control `2025` 12.00); `Test_run_holdings_as_of_the_current_year_or_month_or_today_is_today` (`2026`, `2026-03`, `2026-03-12` → caption 2026-03-12, 250 sh, price 15.00, exit 0, stderr empty); `Test_run_holdings_refuses_a_bad_as_of_before_looking_for_a_store` (empty `HOME`, `2024-13` → exit 2 not 1, the S.4 line); `run_usage_test.go:235` add row `holdings --as-of` → `quarry: flag needs an argument: --as-of; Run 'quarry holdings --help' for usage.` (the table at `:212-222` is `--currency`-only; add a `--as-of` sibling loop only for holdings). `-json` as_of cell is covered in step 4

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ParseAsOf` and `AsOfError` (exported budget ~4 lines)

### Verify
- [ ] Step 7: full verification block per `.claude/rules/agent-briefs.md` + `.claude/scripts/spec-check.py phase4b-holdings`; tick SCENARIO-05 with its acceptance test and SCENARIO-11 as `delivered by SCENARIO-05 — <its test>` (test reference last on the line); rewrite STATE.md; `status: done`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `report.ParseAsOf(value, now)` is the one as-of parser — S13's MCP `as_of` calls it and words `AsOfError` itself (`Kind`, `Value` are parts, not wording); a distinct type from `WindowError`, so no `mcp/window.go` case is added
- A year or month containing today resolves to today and is never refused; only a period wholly after today is refused, naming its resolved last day (`2099` → `2099-12-31`) — S.4's `2099-01-01` row is a full date
- The CLI parses `--as-of` before `currency.resolve` and before opening the store: a bad value exits 2 with no store and no config read, and an empty `--as-of ""` is "not a date" (whitespace is not trimmed)
- No `--as-of` ⇒ `report.Today(now())` (flag `Changed`, not empty-string, decides)

**Left unbuilt** — named so nobody assumes it exists:
- `--account` and `account_filter` — SCENARIO-10; MCP `as_of` argument — SCENARIO-13; the no-price / no-rate / empty-day warnings that a past day can trigger (day before first trade, before first rate) — SCENARIO-06/08/09

**Traps** — things that look right and are not:
- Test clocks and fixed as-of days must not be after the real date (DuckDB `current_date` arm); `holdingsClock()` is 2026-03-12, so `--as-of 2026` resolving to today is safe, but a "tomorrow" row must derive from the clock, never from the real date
- `parseDateBound` (`report/window.go:139`) builds a `WindowError`: extract the layout loop, do not wrap its error and re-label `Bound`
- `time.Parse` rejects `2025-02-29` and `2025-1`, which is the intended not-a-date set; do not add trimming or lenient layouts
- Help tests pin by regexp per flag line (STATE.md): add the `--as-of` regexp, never replace the whole-help pin
