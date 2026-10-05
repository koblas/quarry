---
id: SCENARIO-15
status: open
---

# SCENARIO-15: Net worth refuses impossible dates (absorbs SCENARIO-11, net worth on a past day)

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_networth_as_of_test.go` `Test_run_networth_refuses_a_future_as_of_a_future_since_and_as_of_with_since`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_networth_as_of_test.go` `Test_run_networth_values_every_counted_account_on_the_as_of_day`
Narrow loop: `go test ./internal/report/ ./internal/mcp/ ./internal/cli/ -run 'AsOf|Window|NetWorth|networth|Holdings'` then `go test ./cmd/quarry/ -run 'networth|holdings_as_of|Test_every_quarry_name'`
Mutation checks: refusals run before any store read (`internal/cli/networth.go` RunE: move the parse after `openReport`) → the no-store-needed row of the acceptance table; future-since refusal in `ParseMonthEndWindow` → `Test_parse_month_end_window_refuses_a_since_after_today`; holdings noun default (`AsOfError.Error`) → `Test_run_holdings_refuses_a_date_it_cannot_use`; as-of x since/until conflict → its acceptance row
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (report; cli/cmd wiring, mcp compile-only, SKILL.md)

Source read: STATE.md (binding), no prior SCENARIO files. Surface: `go doc`/Read of `report/asof.go`, `window.go`, `cli/networth.go`; callers of `ResolveAsOf`/`ParseAsOf` (grep, `-v _test`): `cli/holdings.go:48`, `mcp/holdings.go:20`, tests only otherwise. `WindowErrorKind` switch sites (grep): `report/window.go:42-53`, `mcp/window.go:26-43` (exhaustive linter).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_networth_as_of_test.go` (new) — two cmd-slice tests via `runWith` + `spendEnvAt(..., holdingsClock())` (today 2026-03-12). (a) refusal table, each exit 2, empty stdout, exact stderr from Surface & Copy refusal table, and each with NO store seeded (proves refusal precedes any store read): bad `--as-of "2024-13"`; `--as-of` day / month / year after today (value quoted as typed; "net worth is valued up to today only"); `--since 2027-01` alone and with `--until 2028` (both "--since 2027-01 is after today; net worth is valued ..."); `--as-of` with `--since`, with `--until`, with both (conflict line); controls: `--since 2026-03` (since month contains today) and `--as-of 2026` (current year → today) exit 0 on a seeded store. (b) SCENARIO-11: seed with transactions on both sides of 2025-12-31 (reuse `seedNetWorthHistoryStore` style via `spendRows`/`replaceStoreWithRates`, adding 2025 rows, a brokerage with a holding priced before and after the day, a closed account, and a USD account with a rate) → `--as-of 2025-12` prints `Net worth on 2025-12-31, amounts in CAD` with balances as of that day only; same output for `--as-of 2025-12-31`; plus `--json` `as_of` = `2025-12-31`, `since`/`until` null, one `dates` entry. Must fail at assertion (unknown flag today).

### Build
- [ ] Step 2: `internal/report/asof.go:8-54`, `cli/holdings.go:48`, `mcp/holdings.go:20` — `AsOfError` carries a noun, `ParseAsOf`/`ResolveAsOf` take it (holdings: "holdings are", networth: "net worth is"); holdings bytes stay identical. Tests in `asof_test.go:36,100-125`: networth wording row, holdings row unchanged, not-a-date line noun-free; sweep existing callers. `mcp/holdings.go:73-79` `asOfWording` stays holdings-only (S17 owns net_worth wording).
- [ ] Step 3: `internal/report/window.go:11-25,40-56,128-147` — new kind `WindowNetWorthSinceAfterToday` (name free) + `Error()` line `--since X is after today; net worth is valued up to today only, so pass an earlier --since`; `ParseMonthEndWindow` raises it for `since` after today AFTER the since-after-until check (explicit `--since 2027 --until 2024` stays "after --until"), with or without until. Replace `window_test.go:476-495` (`..._accepts_a_since_after_today_...`) with `Test_parse_month_end_window_refuses_a_since_after_today` (alone, later until, value quoted as typed; control: since = today and since = month containing today accepted; precedence row vs since-after-until). Update `ParseMonthEndWindow` doc. `mcp/window.go:26-43`: add the case (bare "since", "net worth is valued up to today only, so pass an earlier since") + row in `mcp/window_internal_test.go:~28-45`; S17 re-pins.
- [ ] Step 4: `internal/cli/networth.go:12-82` — `--as-of` flag with Surface & Copy help verbatim (placeholder `date`); RunE: conflict check first (any of `--as-of` Changed with `--since`/`--until` Changed → `UsageError`, ruled line), then `ResolveAsOf` (given-pointer like `holdings.go:44-48`; `--as-of ""` is given and not a date) into `request.AsOf` with nil `Window`; all before `openReport`. Delete the S12 interim: `render_networth_history.go:76-88` `netWorthHistoryCaption` window fallback (caption always from first/last listed month end), `render_networth_internal_test.go:215-230`, `run_networth_history_test.go:177-203` (both since-after-today tests, superseded by acceptance (a)). Add the `--as-of` help line to `run_networth_surfaces_test.go:48-52` (verbatim, wrapped as cobra prints it); `internal/cli/networth_test.go`: conflict/bad-value refused with `refusedEnv` (UsageError, store never opened). Spec `specification.md:191` interim bullet: drop the "(interim ...)" clause. SKILL row (same batch, tiny): `plugin/skills/quarry/SKILL.md:46-47` add `| Net worth today, on a day, or by month | \`quarry networth [--as-of <d> \| --since <d>] --json\` |` verbatim after the Account balances row; drift test `run_skill_drift_test.go:290` must see both flags (run it, and verify it reddens if `--as-of` is removed from the command, as the control).

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `AsOfError` noun, new kind, `ParseMonthEndWindow`; `golangci-lint`'s exhaustive linter lists any switch missing the new kind.

### Verify
- [ ] Step 6: full verification (`agent-briefs.md` → Verification) + `.claude/scripts/spec-check.py phase4c-networth`; tick SCENARIO-15 and SCENARIO-11 (line for 11: "delivered by SCENARIO-15" before its test ref, test ref last); rewrite STATE.md (drop the S15 Left-unbuilt items and the empty-caption open debt; trap 27's "since after today is accepted" is now false); `status: done`.

## Handoff

**Binding decisions:**
- Networth refusals are all `UsageError` (exit 2) raised before `openReport`; a refusal never reads the store — S16 (before-data warning) and S14b must not move parsing after the open.
- `ParseMonthEndWindow` is the only place networth's future-since refusal lives, after since-after-until; since never exceeds the clamped until afterwards, so a history always lists at least one date from the CLI. `monthEnds`' inverted-window guard stays (public `Window` into `Server.NetWorth`; MCP S17 builds windows through the same parser).
- `AsOfError` noun: holdings keeps "holdings are valued up to today only" (pinned at cmd `run_holdings_as_of_test.go:150-158`, mcp `holdings_test.go:177`, `run_mcp_holdings_test.go:87`); networth says "net worth is valued up to today only".
- `--as-of`: snapshot only, `Window` nil, JSON `as_of` set, `since`/`until` null (S10 binding). Conflict is tested on Changed flags, so `--since ""` still conflicts.
- Precedence of conflict vs bad values is unruled: conflict is checked first (needs no parsing); pinned by one acceptance row.

**Left unbuilt:**
- MCP `net_worth` as_of/since/until wording and `asOfWording` noun — SCENARIO-17 (S15 only adds the `windowWording` case so the exhaustive switch compiles).
- Warnings, `no rate`, empty-history warning — 14a/14b/16 (unchanged).

**Traps:**
- `ParseAsOf` clamps a current month/year to today but refuses one that BEGINS after today (`2026-11` on 2026-03-12): the "current year/month" control row is `2026` / `2026-03`.
- `--since 2027-01` with no store seeded must still exit 2, not 1; a test that seeds a store cannot prove the ordering.
- Adding the window kind without a `mcp/window.go` case fails the exhaustive linter, not `go build`.

## Phase report

Run A (done). Added `cmd/quarry/run_networth_as_of_test.go` only; no production change, no stubs needed (no new symbol).
- `Test_run_networth_refuses_a_future_as_of_a_future_since_and_as_of_with_since` (acceptance): 11 refusal rows (no store seeded, exit 2, empty stdout, exact stderr; incl. empty `--as-of ""` and the conflict-before-bad-value row `--as-of 2024-13 --since 2026-01`) plus 2 controls on `seedNetWorthHistoryStore`.
- `Test_run_networth_values_every_counted_account_on_the_as_of_day` (SCENARIO-11 fold): `--as-of 2025-12` and `2025-12-31` over `seedNetWorthAsOfStore` -> `Net worth on 2025-12-31, amounts in CAD`, brokerage CAD 920.00, chequing CAD 1,125.00 (before + on-day, after excluded; closed account 25.00 counted), chequing USD 800.00 -> 1,088.00 at the 1.36 rate (a later 1.50 rate must not apply), Total 3,133.00.
- `Test_run_networth_json_as_of_sets_the_day_and_leaves_since_and_until_null`: extra, `as_of` 2025-12-31, since/until null, one `dates` entry, CAD total 3133.00.
Red: all 11 refusal rows fail at their assertions (`unknown flag: --as-of` stderr; `--since 2027-01` rows exit 1 "no store"), `--as-of 2026` control fails (exit 2 unknown flag), both as-of snapshot subtests and the JSON test fail at `require.Equal(0, exitCode)` (actual 2). Green on arrival: control `--since 2026-03` (works today by S12; kept as the future-since arm's control).
Not yet verified: the seed's expected 3,133.00 / 920.00 are computed by hand; B1 run confirms them when `--as-of` lands. If a figure is off, check the seed arithmetic first (see seed doc comment), not production.
Next: B1 steps 2-4 (AsOfError noun, new window kind, networth flag + conflict; delete S12 interim caption tests; SKILL row).
