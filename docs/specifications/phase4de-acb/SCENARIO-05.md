---
id: SCENARIO-05
status: open
---

# SCENARIO-05: Finding counts include unclassified accounts (absorbs SCENARIO-03)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_status_unclassified_test.go` `Test_run_status_counts_an_unclassified_account_until_the_config_classifies_it`
Acceptance test (SCENARIO-03, folded): `cmd/quarry/run_accounts_unmatched_test.go` `Test_run_accounts_and_findings_warn_a_listed_id_that_names_no_account`
Narrow loop: `go test ./internal/report/... ./internal/store/... ./internal/snapshot/ ./internal/importer/ ./internal/cli/ ./internal/mcp/ ./cmd/quarry/ -run '(?i)unclassified|unmatched|count|status|sync|accounts|data_quality'`
Mutation checks: delete the read-time option at `internal/cli/sync.go:102` → `Test_run_sync_counts_an_unclassified_account_open_and_never_new`; compute unmatched ids after the closed filter in `(*report.Server).Accounts` → `Test_accounts_warns_nothing_for_a_listed_closed_account`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (report; `snapshot` option argument, duckstore adapter and a one-field `internal/importer` pass-through at importer.go:177-181 do not count)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_status_unclassified_test.go` (new) `Test_run_status_counts_an_unclassified_account_until_the_config_classifies_it` — per-test v9fixture: one brokerage account + chequing control, `syncBundle`, no config → `quarry status` Findings row `1 open; run quarry findings to list them`; then `writeConfig` lists the id in `accounts.non-registered`, status again WITHOUT sync → `none open`. Must fail at the first assertion (`none open` today)
- [x] Step 2: `cmd/quarry/run_accounts_unmatched_test.go` (new) `Test_run_accounts_and_findings_warn_a_listed_id_that_names_no_account` — rows `accounts`, `findings`; config `registered = ["acct-99"]`, `non-registered = ["RBC 12345678"]`; stderr holds verbatim `quarry: warning: ~/Library/Application Support/quarry/config.toml: accounts.registered lists "acct-99", which is not an account in quarry's store; quarry skips it` and the `accounts.non-registered lists "RBC ****5678"` line; exit 0. Signature-only stubs not needed (no new symbol referenced)

### Build
- [x] Step 3: `internal/store/store.go:355-369` `Status.Accounts []Account` (+ doc); `internal/store/duckstore/status.go:45-100` reads accounts in the same open — extract the account scan at `findings_read.go:93-103` (`findingAccountsQuery` :39) into one helper both reads call; `internal/report/findings.go:91-96` `CountFindings(st, ignore, c Classification)` builds `store.FindingList{Findings: st.Findings, Accounts: st.Accounts}` and goes through `readTimeFindings` (readtime.go:12) before `knownFindings`. Tests: duckstore Status returns accounts (closed included, sorted by id) + fault test: account query fails → `*store.OpenError`; `internal/report/finding_counts_test.go:14` helper gains classification; rows: unclassified counted open, never `New`; listed registered / non-registered → not counted; non-investment unlisted → not counted; ignored unclassified id → `Ignored`; unknown stored type still dropped
- [x] Step 4: callers — `internal/cli/status.go:40-42,55-64` `statusIgnore` also returns the classification (empty when config unreadable); `internal/mcp/sync_status.go:23-35` same. Hoist the `report.Classification{Registered: cfg.Registered, NonRegistered: cfg.NonRegistered}` spelling into one unexported helper per delivery package: cli (`currency.go:82-95` beside `readConfig`; sites `accounts.go:48`, `findings.go:108`, status, sync) and mcp (`data_quality.go:33`, sync_status). Pins (each with control = same fixture, id in `accounts.non-registered`): status `--json` `findings.open`/`findings.new` (`run_status_json_test.go`), MCP `sync_status` `findings.open` (`cmd/quarry/run_mcp_status_test.go:34`), ignored-unclassified id → `findings.ignored` at status. Unreadable config → every investment account counted open: add the row at `run_status_findings_test.go:115`, `:145` and `internal/mcp/sync_status_test.go:122`; re-pin `sync_status_test.go:64`
- [x] Step 5: sync — `internal/store/store.go:395-417` `Result.Accounts []Account` (doc: the accounts the build wrote); `internal/importer/importer.go:177-181` passes `accounts`; `internal/snapshot/snapshot.go:30-41,108-112` new option beside `WithIgnore` taking `func(store.FindingList) []finding.State`, applied at `import.go:192` (states appended before `finding.Classify`, `ImportFrom` shares the path); exported report func/method returning those states via `readTimeFindings` + `knownFindings` (never New); wire at `internal/cli/sync.go:102`. Tests: snapshot unit with fake importer (option unset → counts unchanged; set → open+1, New unchanged; ignore applies to a read-time id); importer test `Result.Accounts` equals the rows written; `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_counts_an_unclassified_account_open_and_never_new` (first sync line `1 open; …`, second sync no `(1 new)`, control listed → `none open`), a `sync --from` row beside `run_sync_findings_test.go:359` (ImportFrom arm), sync `--json` `store.findings.open`/`new`, ignored row; one agreement test `Test_run_status_sync_and_mcp_agree_on_an_unclassified_account_count` (sync line, status, `sync_status` `findings.open`, `data_quality` `counts.open` equal on one fixture)
- [x] Step 6: SCENARIO-03 warning — `internal/report/classification.go:11-35` method returning the ids of each list naming no store account (file order, duplicates kept, every account type, closed included); `report/findings.go:44-50,53-89` `FindingsListing` field set from `list.Accounts` (independent of `--type`, like :68); `report/accounts.go:15-25,53-74` `AccountListing` field computed BEFORE the closed filter at :62-71; `internal/report/document/findings.go:117-126` new composer beside `UnmatchedIgnoreWarnings`: `accounts.<registered|non-registered> lists ` + `tomlstr.BasicString(accountmask.Mask(id))` + ruled tail. Wiring: `cli/findings.go:117-119` after the unmatched-ignore lines (stderr `~`, JSON absolute); `cli/accounts.go:48-68` stderr `~` via `printConfigWarnings` after the listing read, absolute joins `withConfigWarnings`' config slot — never the `warnings` slice (it feeds `renderAccountsJSON`); `mcp/data_quality.go:40` absolute after the unmatched-ignore lines. Tests: report rows (unmatched registered, unmatched non-registered, duplicate kept twice, matched non-investment → none, `Test_accounts_warns_nothing_for_a_listed_closed_account` without `--all`, `--type` filter keeps the warning); document composer rows (`acct-99` unmasked, `12345678` → `"****5678"`, quote in id escaped); cli/cmd: `accounts --json` and `findings --json` `warnings[]` absolute path, MCP `data_quality` `warnings[]` absolute (`cmd/quarry/run_mcp_data_quality_test.go`); silence rows at `run_status_findings_test.go:188`, `run_sync_findings_test.go:375`, MCP `sync_status` — each runs the same config through `quarry accounts` (or `findings`) as control and asserts the warning DOES appear there
- [x] Step 7: fixture re-pins per the STATE fixture rule (subject is counts → re-pin; else local `accounts.non-registered` or `--type`; never `syncBundle`) — run the narrow loop over `./cmd/quarry/ ./internal/mcp/` and fix what fails; known candidates `cmd/quarry/run_status_test.go:57`, `run_validation_test.go:96`, `run_shared_documents_test.go:177,199`, plus sync/status goldens with brokerage/retirement fixtures (`run_success_test.go`, `run_import_test.go`, `run_json_test.go`, `run_investment_cash_test.go`)

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `CountFindings`, `Status`, `Result`, the new option, report func and composer; no Long/help copy changes (counts reuse existing lines)

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-05 with its acceptance test; tick SCENARIO-03 `— delivered by SCENARIO-05 —` then its acceptance test last on the line; rewrite STATE.md

## Handoff

**Orchestrator: copy ruling 2026-10-05 (product-vision) on action 1 — recorded in spec Part A.** Behaviour as planned (unreadable config → every investment account counts as open unclassified). Warning at `internal/report/document/status.go:109` becomes `cannot tell which findings you ignored or how you classified your accounts: <problem>; findings you ignored are counted as open, and every investment account is counted as unclassified`; rename `CannotTellIgnored` → `CannotTellChoices`; update comments at :97-99, :107. Pins: `internal/report/document/status_test.go:33-36`; `cmd/quarry/run_status_findings_test.go:66-67,72-73`; `cmd/quarry/run_shared_documents_test.go:58-59,189`; `internal/mcp/sync_status_test.go:137,150` (rename). Done in Step 4.


**Binding decisions** — a later scenario must not contradict these without saying so:
- Read-time inputs have THREE sources: `store.FindingList` (duckstore `Findings`), `store.Status.Accounts` (duckstore `Status`), `store.Result.Accounts` (importer) — 13b must add its detector input to all three or status/sync silently disagree with findings
- Every count site goes through `readTimeFindings`: `CountFindings(st, ignore, c)` for status/sync_status; sync via the snapshot option taking `func(store.FindingList) []finding.State` — snapshot cannot import report, so 13b only adds a `FindingList` field, never a second option
- Unreadable config at `status`/`sync_status` → empty Classification → every investment account counted open (matches `run_status_findings_test.go:115` "counts every finding open")
- Unmatched account ids: computed in `report` over every store account (any type, closed included) before any filter; composed in `document`; mask then quote; duplicates kept; stderr order after unmatched-ignore lines; `status` and `sync` stay silent
- One unexported Classification helper per delivery package (cli, mcp); no literal `report.Classification{Registered: cfg…}` elsewhere

- Pending product-vision copy ruling (behaviour above ships; only wording open): `document.CannotTellIgnored` (document/status.go:109) says only ignored findings count open, not that classified accounts count unclassified — orchestrator, before B1

**Left unbuilt** — named so nobody assumes it exists:
- `shares-without-cost` read-time detector and its inputs — S13b

**Traps** — things that look right and are not:
- The sync-vs-status agreement test is what proves importer account rows (`Type` especially) equal the stored ones; do not drop it as redundant
- `Server.Accounts` without `--all` filters closed accounts (accounts.go:62-71): an unmatched check after it warns a real closed account
- `accounts` reuses one `warnings` slice for stderr AND `renderAccountsJSON`; a `~` line appended there leaks into `--json`
- Read-time states must never set `New`: sync's `(N new)` clause would then count every unclassified account on every sync

## Phase report

Run B2 (steps 6-7) done and committed. `go test ./internal/... ./cmd/...` green (rc=0), `golangci-lint run ./...` 0 issues (lint already clean, incl. two B1 leftovers: `lll` on `run_shared_documents_test.go` status-unreadable const, `wrapcheck` on `duckstore/findings_read.go` `readAccounts`). Both acceptance tests green.

Production: `report/classification.go` `UnmatchedAccounts` + `(Classification).Unmatched`; `FindingsListing.UnmatchedAccounts` (from `list.Accounts`), `AccountListing.UnmatchedAccounts` (computed before the closed filter, `accountsOf` helper in `report/accounts.go`); `document.UnmatchedAccountWarnings` (`document/findings.go`); wired at `cli/findings.go` (stderr `~` after unmatched-ignore, JSON absolute), `cli/accounts.go` (stderr `~` before own warnings via `printConfigWarnings`; JSON absolute in `withConfigWarnings`' config slot), `mcp/data_quality.go` (also copied through `capFindings`/`capItems`, which rebuild `FindingsListing`).

Tests: `internal/report/unmatched_accounts_test.go` (7), `internal/report/document/unmatched_accounts_test.go` (5), `cmd/quarry/run_accounts_unmatched_json_test.go` (6: accounts/findings --json, closed account silent, MCP data_quality, status+sync silent, MCP sync_status silent; each silence row has `accounts --json` control).

Step 7: `Test_run_checks_balances_and_split_sums_before_swapping_the_store_in` failed because its fixture holds an unclassified brokerage (`Findings 1 open` -> 2): expected consequence, not a defect; subject is validation so it gets a local `accounts.non-registered` config. Re-pinned counts 2 -> 3 in `run_status_test.go`, `run_status_json_test.go`, `run_shared_documents_test.go` (status unreadable-config golden).

Mutation (plan line 2): `report/accounts.go` recompute `unmatched` after `list.Accounts = open` -> `Test_accounts_warns_nothing_for_a_listed_closed_account` red (`Should be empty, but was [acct-1]`); restored, diff clean.

Left for V: full covered run + uncovered-diff, test-stats, spec tick (SCENARIO-05 + folded SCENARIO-03), spec-check, STATE.md rewrite, status: done, doc-comment check on new symbols.
