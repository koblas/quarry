---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Accounts show their classification

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `internal/cli/accounts_classification_test.go` `Test_accounts_show_their_classification`
Narrow loop: `go test ./internal/config/ ./internal/report/ ./internal/cli/ -run 'Account|Config|Classif|Currency|Conventions'`
Mutation checks: none
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches (config, report, cli, conventions sentence), 2 feature packages (config, report; cli/cmd wiring and `platform` text do not count)

Existence facts (LSP/grep): no `Registered`/`NonRegistered` anywhere; `Server.Accounts` has one production caller (`cli/accounts.go:40`), 10 test callers (`report/accounts_test.go` 6, `refusal_test.go` 2, `accounts_unvalued_test.go` 2); MCP has no accounts tool. No new port: classification is a value passed in. Config reaches `accounts` only through `currencyFlag.resolve` (`cli/currency.go:56-72`), which does NOT read the loader when `--currency` is given.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/cli/accounts_classification_test.go` `Test_accounts_show_their_classification` — `cli.Execute` accounts, fake report store (`fakeReportStore`, `accounts_test.go:30-40` pattern), a `LoadConfig` returning `config.Config{Registered, NonRegistered}`; rows: listed-registered brokerage, listed-non-registered, unlisted brokerage, unlisted chequing, listed-registered chequing. Text: Status `registered` / (none) / `unclassified` / (none) / `registered`; `--json`: `"registered"` true / false / null / null / true, placed after `linked_tracking`
- [ ] Step 2: stubs so it compiles and fails at its assertion — `config.go:21-36` `Config.Registered`, `Config.NonRegistered` (`[]string`, nil when unset); `report/accounts.go` `Classification` type + extra param on `(*Server).Accounts` + rule method (unimplemented); fix the 10 test call sites with `report.Classification{}`

### Build
- [ ] Step 3: `config/parse.go:34-54,56-95,223-243,316-325` — `accountsRegisteredSetting`/`accountsNonRegisteredSetting` (table `accounts`, example `accounts.registered = ["acct-12"]` so `lookup`'s existing "must be a table, such as …" line is the ruled one); generalise `ignore()` into one id-list parser shared with the new keys (noun `account ids`, example `["acct-12"]`); parse after `currency()`, before unknown keys; knownKeys += `{accounts}`, `accounts.registered`, `accounts.non-registered`; update `Load`/`parse` doc order. Tests `config/accounts_test.go`: both lists load in file order, spelling and duplicates kept; unset → nil; non-`acct-` text kept as written (S03 warns); `accounts.registred` → unknown-key warning (both `Warnings` and `WarningsAbsolute`); `[accounts]` table form and dotted form both load. Fault rows (interim, see Handoff): non-list value, non-string item (as item N), `accounts = 5` (table-as-value) — each refused with the ruled text, exit via existing `Load` error
- [ ] Step 4: `report/accounts.go:13-22,50-70` + new `report/classification.go` — `Classification{Registered, NonRegistered []string}`; `AccountListing` carries it; one exported method returns tri-state (true listed registered / false listed non-registered / nil neither) and one reports "unclassified" = `store.IsInvestmentAccount(a.Type)` and neither list. Tests via `Server.Accounts` + `fakes_test.go`, one row per arm: registered; non-registered; investment unlisted (brokerage and retirement each); non-investment unlisted → nil and not unclassified; non-investment listed registered → true; closed, not-in-reports and linked-tracking investment unlisted → unclassified; zero holdings; two accounts same name → each own class; listed in both → registered wins (interim); `--all` hidden-closed count unchanged

### B2
- [ ] Step 5: `cli/accounts.go:17-27,29-33,50-60`, `cli/currency.go:56-72`, `cli/render_accounts.go:20,80-96`, `cli/json_accounts.go:15-55` — accounts always loads the config (flag or not), prints its warnings once, builds `report.Classification` from it, passes it into `srv.Accounts`; `accountStatus` appends `registered` / `unclassified` last (`closed, not in reports, registered`); `accountRowDocument` gains `Registered *bool \`json:"registered"\`` right after `LinkedTracking`; Long gains the ruled paragraph (Surface & Copy Part A → accounts → Long) after the "quarry does not add balances together here" paragraph, verbatim, wrapped like its neighbours. Tests: edge rows × text and `--json` × `--all` (closed unlisted shows only with `--all`); Long pinned verbatim at wrap width (find the existing accounts-help pin; `report_help_test.go` is the family); `accounts --currency native` still reads the loader (move `accounts` out of `currency_test.go:164-193` "only without the flag" and pin it separately); loader error with the flag → exit 1 runtime error; Status order against `inactive`/`not in reports`/`linked tracking`
- [ ] Step 6: `report/sql_conventions.go` (end of the last paragraph, after "no exchange rate for that day.") — add verbatim: "Which accounts are registered is not in the store; it is accounts.registered and accounts.non-registered in quarry's config, and quarry accounts --json reports it as registered." Then regenerate `plugin/skills/quarry/references/schema.md` (`go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`) and update the hand copies `internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`, `internal/report/sql_conventions_test.go`; `describe_schema` and `quarry sql --help` share `report.SQLConventions`, so one pin per surface already exists — update them, add no new copy

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Classification`, the tri-state method and the two `Config` fields; golden re-pins (below)

### Verify
- [ ] Step 8: full verification block + `.claude/scripts/spec-check.py phase4de-acb` → tick SCENARIO-01 with its acceptance test; write `STATE.md`

Golden re-pins (trap): every accounts `--json` fixture gains `"registered": null`; every fixture with a brokerage/retirement account and no `[accounts]` gains `unclassified` in Status. Search with `go test ./cmd/quarry ./internal/cli -run 'Account|Holdings|Currency|Golden'`; known files: `internal/cli/json_accounts_internal_test.go`, `json_accounts_wide_internal_test.go`, `render_accounts_internal_test.go`, `cmd/quarry/run_accounts_*_test.go`, `run_analysis_documents_golden_test.go`, `run_currency_native_test.go`, `run_config_test.go`. Re-pin only diffs that are exactly those two additions; any other diff is a bug.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Classification is passed INTO `report.Server.Accounts` as a `report.Classification` value; `report` owns the rule (registered true/false/nil; unclassified = investment type and in neither list) — S04/S05 reuse the same method for the finding and counts so every site agrees (R-1).
- `Config.Registered` / `Config.NonRegistered` keep file order, duplicates and spelling (`findings.ignore` precedent) — S03's unknown-id warning and S02's "in both" refusal read them as written.
- `accounts` always loads the config now (even with `--currency`); the other six `currencyFlag` commands still skip the loader when the flag is given (`currency_test.go` pin) — classification has no other route to `accounts`.
- Status words append last, after `linked tracking`; JSON `registered` sits after `linked_tracking`.
- The registered-is-not-in-the-store sentence ends `report.SQLConventions`; S06's cost_basis sentence and S19's go in the investment paragraph / after it, not before this one.

**Interim behaviour (what a malformed `accounts.*` does until S02)** — no crash, never silent: refused with the ruled shape lines (`accounts.<X> must be a list of account ids in quotes, such as ["acct-12"], got <v>`; `… must hold only account ids in quotes, got <v> as item <n>`; `accounts must be a table, such as …` comes free from `lookup`), but `<v>` is echoed UNMASKED and the same id in both lists is NOT refused (registered wins). Tests here use values of four characters or fewer so masking leaves them unchanged.

**Left unbuilt** — named so nobody assumes it exists:
- `Mask`-style helper in `internal/platform`, masking at the refusal "got"/"as item" text and the unknown-key warning under `[accounts]`; the "in both" refusal; go-toml syntax-error echo probe (`parse.go:104`) — S02. S02 is now 2 batches (helper + sites, in-both); orchestrator may FOLD it into the next config scenario.
- `accounts.* lists "<v>", which is not an account in quarry's store` warning — S03, folded into S05.
- `unclassified-account` finding type, `Types()` entry, status/sync/MCP counts — S04/S05.
- `cost_basis` column, FormatVersion 9, cost_basis conventions sentence — S06.

**Traps** — things that look right and are not:
- `currencyFlag.resolve` returns early with the flag: reusing it alone gives `accounts --currency native` an empty classification (every investment account `unclassified`) — wrong silently.
- `Server.Accounts` filters closed accounts after the read; classification must be applied to the rows that remain, and `Hidden` stays a count of closed accounts only.
- A non-investment account in neither list has Status nothing and JSON `null` (not `false`); only a listing in `accounts.non-registered` is `false`.
- `schema.md` is generated: edit the const and run the `-update` test, never hand-edit (header says so).
