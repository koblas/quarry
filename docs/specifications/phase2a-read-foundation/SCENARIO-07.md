---
id: SCENARIO-07
status: open
---

# SCENARIO-07: accounts --json returns accounts as a document (absorbs SCENARIO-06, the all-closed note)

Size verdict: OWNS A RUN (one behaviour, `accounts --json`; SCENARIO-06 folded in — its note and `warnings[]` are the same command's output and the JSON `warnings[]` needs it).
Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_accounts_json_test.go` `Test_run_accounts_json_returns_accounts_as_a_document`
Acceptance test (SCENARIO-06, folded): `cmd/quarry/run_accounts_test.go` `Test_run_accounts_says_how_to_list_them_when_every_account_is_closed`
Narrow loop: `go test ./internal/report/ ./internal/cli/ -run 'Accounts|AllClosed' && go test ./cmd/quarry/ -run 'accounts'`
Mutation checks: `AllHidden`'s `len(Accounts)==0` half → `Test_accounts_reports_every_account_hidden_only_when_none_are_left` (mixed row); `Hidden>0` half (zero accounts, no note) and the exactly-1 copy branch → `Test_accounts_all_closed_note` rows `zero accounts` / `exactly one`

## Implementation Plan

Contract. `quarry accounts [--all] [--json]`, exit 0. Stdout: human table, or the JSON document `{as_of, accounts[{id,name,type,currency,institution,closed,active,balance}], warnings}` (2-space indent, trailing newline; `warnings` always present). All accounts closed and no `--all`: stdout is header only / `accounts:[]`; stderr `quarry: all 3 accounts are closed; pass --all to list them` (exactly 1: `quarry: the only account is closed; pass --all to list it`), same text minus `quarry: ` in `warnings[]`, in BOTH modes. Zero accounts or `--all`: no note, `warnings:[]`. Store fault / stdout write fault: exit 1, existing mapping, no note. Rule lives in report (`Hidden`, `AllHidden`), copy in cli.

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_accounts_json_test.go` (new) `Test_run_accounts_json_returns_accounts_as_a_document` — fixture: checking with an institution (`v9fixture` `Institution` row, builder.go:242), retirement with a transaction, an account with no institution; assert `as_of` is a local date taken around the `run` call (accept the day before or after, midnight race), balance string / `null`, institution `null`, `warnings:[]`, stderr empty. Compiles today, fails at the assertion (prints the table)
- [ ] Step 2: `cmd/quarry/run_accounts_test.go:116-145` (append) `Test_run_accounts_says_how_to_list_them_when_every_account_is_closed` + `syncClosedAccountsFixture(t, home, n)` — 3 closed accounts, human mode: stdout header only, stderr the N≥2 line, exit 0. Fails today at the stderr assertion

### Build
- [ ] Step 3: `internal/report/accounts.go:9-29` `AccountListing{store.AccountList; Hidden int}` + `(AccountListing).AllHidden() bool` (`Hidden>0 && no accounts left`); `(*Server).Accounts` returns it, `Hidden` = closed accounts filtered (0 when `includeClosed`). Update `accounts_test.go:13-50`: `Test_accounts_leaves_closed_accounts_out_unless_asked` compares the new type; add `Test_accounts_counts_the_closed_accounts_it_left_out` and `Test_accounts_reports_every_account_hidden_only_when_none_are_left` (all closed / mixed / zero accounts / `--all` over all closed). Store-fault test at :44 stays
- [ ] Step 4: `internal/cli/json.go:118-135` + `json_status.go:72-83` — third encoder copy is coming: extract one `marshalDocument(doc any) ([]byte, error)` (indent, newline, the single `// unreachable:` line) used by sync, status and accounts; behaviour-neutral, existing `Test_renderJSON`/`Test_renderStatusJSON` cover it
- [ ] Step 5: `internal/cli/json_accounts.go` (new) `accountsDocument`, `accountRowDocument`, `renderAccountsJSON(store.AccountList, warnings []string)` — `as_of` = `AsOf.Format(jsonDateLayout)` as-is (never `.In(time.Local)`); balance via `jsonMoney` or nil; `Institution` passed through; `accounts` built with `make` so zero accounts is `[]`, never `null` (duckstore returns a nil slice for zero accounts). `json_accounts_internal_test.go` `Test_renderAccountsJSON`: full-document golden (grouping-free, negative `-1204.17`, `0.00`, `null` balance, `null` institution, closed/inactive flags), empty list, warnings carried, `as_of` unchanged under a negative-offset `useZone` (render_status_internal_test.go:14; not in a `t.Parallel` test)
- [ ] Step 6: `internal/cli/accounts.go:9-39` + `root.go:29` — `newAccountsCommand(newReport, jsonOut *bool)` (root passes `jsonOut`, as status does); `allClosedNote(hidden int) string` (N≥2 via `accountsPhrase`, exactly-1 copy; unexported, ok to live in `render_accounts.go`); command builds `warnings` (`[]string{}` or the note), renders JSON or `renderAccounts(listing.AccountList)`, writes stdout, then writes `"quarry: "+note` to `cmd.ErrOrStderr()` only after stdout succeeded (ignore that write's error like `sync.go:123`)
- [ ] Step 7: `internal/cli/accounts_test.go` (new, `package cli_test`, `cli.Execute` + `report.NewServer(report.WithStore(fake))` — `report.Store` is exported; no DuckDB) `Test_accounts_all_closed_note`: table over {N=3, N=1204 (`1,204`), exactly one, zero accounts, mixed open+closed, `--all` over all closed} x {human, `--json`} asserting stdout, stderr and `warnings[]` together; `Test_accounts_writes_no_note_when_stdout_fails` (failing writer, error returned, stderr empty); `Test_accounts_returns_the_report_fault` (`newReport` error and `Accounts` error, stdout empty)
- [ ] Step 8: `cmd/quarry/run_accounts_json_test.go` `Test_run_accounts_json_reports_the_all_closed_note_in_both_streams` (3 closed, `--json`: `accounts:[]`, `warnings` has the text, stderr has `quarry: `+text) and `run_accounts_test.go` `Test_run_accounts_all_lists_closed_accounts_without_a_note` (same fixture, `--all`, stderr empty)

### Sweep
- [ ] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols (`go-code.md` budgets)

### Verify
- [ ] Step 10: full verification per `.claude/rules/agent-briefs.md`; `.claude/scripts/spec-check.py phase2a-read-foundation`; tick SCENARIO-07 with its acceptance test and SCENARIO-06 as `— delivered by SCENARIO-07 — ` + its acceptance test (test reference last on the line); `git add` new files before `uncovered-diff.py`; rewrite STATE.md; `status: done`

## Handoff

**Binding decisions:**
- `(*report.Server).Accounts` returns `report.AccountListing{store.AccountList; Hidden int}`; the all-closed rule is `AllHidden()` in report, copy is `allClosedNote` in cli, and P2a-3 reuses `Hidden` rather than counting again — the note counts what report hid, so the filter stays out of SQL (S04).
- Note text is one string: stderr = `"quarry: "+note`, `warnings[]` = `note`; the same in human and `--json`; stdout is never touched by it.
- `marshalDocument` is the one JSON encoder (indent, trailing newline); S09's `sql --json` goes through it.
- `accountsDocument` is distinct from sync's `accountDocument` (5 keys, "never_reconciled"); do not reuse.

**Left unbuilt:** U8 (`accounts` still accepts positional args; S18), U9 hint (S19), R1-R3 refusals (S15-17), O2's ruled stdout-write text (S18/S21), `--all` in JSON keeps no extra key.

**Traps:**
- `nil[:0:0]` is nil: zero accounts reaches cli as a nil slice; a `[]T` built by append encodes `null`.
- `Institution` is a pointer passed through as-is; an importer that stores `""` (not NULL) would print `""` — developer greps the importer's institution mapping, and if `""` is possible maps it to `null` in `renderAccountsJSON` with a test.
- `as_of` is `current_date` (process-local day) at UTC midnight: format as-is; the cmd test must tolerate a midnight rollover.
