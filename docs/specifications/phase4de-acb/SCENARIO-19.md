---
id: SCENARIO-19
status: open
---

# SCENARIO-19: MCP acb

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_mcp_acb_test.go` `Test_run_mcp_acb_returns_the_acb_json_document`
Narrow loop: `go test ./internal/mcp/ ./internal/report/ ./internal/cli/ -run 'acb|cap|conventions|tool|mcp|sql_help'` and `go test ./cmd/quarry/ -run 'mcp|skill|schema|holdings_copy|byte_for_byte'` (names are lowercase snake: `-run` is case-sensitive)
Mutation checks: warnings built from the cut report instead of the uncut one in `(*Server).acb` → `Test_run_mcp_acb_warns_of_securities_the_security_param_leaves_out`; adjustments not mapped from `cfg.Adjustments` → `Test_run_mcp_acb_returns_the_acb_json_document` (adjustment row); event cap `> maxRows` → `>= maxRows` → `Test_acb_caps_events_at_500_across_securities`; `s.now()` read twice → `Test_acb_reads_today_once_per_call`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`internal/mcp`; `report/sql_conventions.go` const, `cli/mcp.go` help string and plugin docs are copy edits, no logic)

No new port, no store change: `report.Server.ACB` already refuses R-6 and unknown `--security`. Surface `internal/mcp` calls: `srv.ACB`, `(ACB).Cut`, `document.ACBWarnings`, `document.NewACB`, `report.ParseACBYear`, `report.Today`, `config.Config.{Path,WarningsAbsolute,Registered,NonRegistered,Adjustments}` (read + `grep`, no LSP; `acbAdjustmentsOf` is in `internal/cli/acb.go:75`, cli-only; mcp keeps its own copy, as `classificationOf` data_quality.go:51 does).

## Contract
Tool `acb`, params `year` (string, absent = none), `security` (string array, absent or `[]` = all); no `currency` (schema rejects it: generic argument refusal). Order: year -> config (`configRefusalLog("acb")`) -> `newReport` -> `Server.ACB` (R-6, then unknown security). Result = `quarry acb --json` document of `Cut()`, warnings from the UNCUT report: `slices.Concat(cfg.WarningsAbsolute, document.ACBWarnings(acb, cfg.Path))`, cap line last. Events capped at 500 in document order (security order, then event order); a later security keeps its header with `events: []`; line `acb lists the first 500 events of N; pass security to narrow` (N thousands-grouped, via the `capList` format).
stderr for a refused param: `quarry: mcp: acb: refused the call's year; details went to the client only` (same with `security`); R-6 stderr = its fixed text.

## Copy not ruled (default, pending ruling) — orchestrator: rule before run A, or accept defaults
1. R-6 on MCP: CLI line verbatim (spec :306 "CLI lines"; `RefusalGeneric`, no new kind). Alt: `data_quality with type unclassified-account and status all lists them` — the CLI line sends Claude to a shell it may not have.
2. Unknown security, client text: `acb covers no security named "XYZ"; call acb without security to list every security it covers` (mirrors `accountRefusal`); `RefusalUnknownSecurity` arm at `result.go:87` becomes the class line `refused the call's security; details went to the client only`.
3. Year, client text (derived from :306): `year "24" is not a year; use YYYY, such as 2024` / `year 2027 is after this year; pass this year or an earlier one`; `year` is a string so `""`, `+2024` stay refusals. Risk: a client sending the integer 2024 is stopped by the schema (generic refusal); alt schema `["string","integer"]`.
4. Param descriptions: `year` = `Tax year to report, YYYY (0001 to this year): years and securities are cut to those with a sale, or a return of capital above ACB, that year. Omit it for every year.`; `security` = `Cover only these securities, each given by id, ticker or name in any letter case; years and securities are cut to them. Omit it for every security.`
5. Warnings reach MCP verbatim, so slot 4 says `quarry findings --type shares-without-cost lists them` and 4b `quarry acb --security <id>`; alt rewords for tools, as `document.NativeParameter` does for net_worth.
6. SKILL §8 R-6 row: `| acb refuses: accounts not classified | exit 1, stderr ¤quarry: acb needs every brokerage and retirement account classified; …¤ | Classify them (¤references/findings.md¤, "Classifying accounts"), then run it again. |`
7. `finding.go:322` sentence lacks LIRA / 401(k) / IRA: default untouched (ruled verbatim :134; pin `finding_test.go:218`). `dataQualityDescription` (`tools.go:77-84`) and `instructions` (`tools.go:42-53`) untouched: final pass owns the first, nothing rules the second.
8. Event cap shape (spec says only "500 via capList"): document order, later securities keep their header with `events: []`, N counted over the cut document; advice when `security` was already given stays `pass security to narrow`. Rule `year`'s type (3) before run A: it changes the acceptance arguments and the schema pin in `run_mcp_descriptions_test.go`. `mcp --help` Tools line: `…holdings,\nnet_worth, acb.`; SKILL §9: `acb` after `net_worth`.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_mcp_acb_test.go` `Test_run_mcp_acb_returns_the_acb_json_document` — `runBothSurfaces` (`run_mcp_documents_helpers_test.go:25-63`) over `acbRows()` (`run_acb_test.go:38`) with `config` classifying acct-cad/usd non-registered, acct-rrsp registered; cases: none, `year`, `security` by ticker, both, and one `[[acb.adjustment]]` config; asserts tool body and warnings equal CLI `acb --json`
- [ ] Step 2: `internal/mcp/tools.go:27-29,317-322` `toolACB`, `acbInput`, registration + `internal/mcp/acb.go` stub `(*Server).acb` returning only `warnings` so step 1 fails at `assert.Equal`; registration reds every all-tools table, so land their rows here: `run_mcp_test.go:43-46`, `run_mcp_descriptions_test.go:217,267` (description + schema consts), `run_mcp_no_store_test.go:34`, `run_mcp_store_faults_test.go:88-96` (directory store; dropped table), `internal/mcp/timeout_test.go:86-93,132` (`stallingStore.InvestmentHistory`), `query_helpers_test.go:42-60` (`fakeStore.InvestmentHistory`). Sweep once with `grep -rn '"net_worth"' internal/mcp cmd/quarry` (positive control: lists the rows above) for any table missed; also check `log_classes_internal_test.go`, `log_internal_test.go`, `run_skill_drift_names_test.go`, `run_mcp_describe_test.go`, `run_mcp_wiring_test.go`, `run_mcp_cancel_test.go`

### Build
- [ ] Step 3: `acb.go` handler, `tools.go:135-147,252-261` param descriptions + `acbInput` + `objectSchema` (ruled description :306, `accountsSchema`-style array), `acbAdjustmentsOf`, `classificationOf` reuse (`data_quality.go:51`); tests in `internal/mcp/acb_test.go` (fake `InvestmentHistory` on `fakeStore` `query_helpers_test.go:42-60`) and `cmd/quarry/run_mcp_acb_test.go`: `Test_run_mcp_acb_warns_of_securities_the_security_param_leaves_out` (warning 4/7 of an unnamed security survives; registered-only slot 2b), `Test_run_mcp_acb_keeps_the_document_key_orders_the_cli_prints` (`years[]` `gain`, `return_of_capital_gain` `"0.00"`, `possible_superficial_losses`; event `unknown_cost` last), `Test_acb_reads_today_once_per_call` (`s.now()` once), year+security together, config warnings first, `Test_acb_reads_the_store_once`. Fault rows for the tables (store: `acb`, store cannot be opened; its table dropped; timeout line `acb stopped after 1 second; try again`) ride step 2. Cross: edge rows x `year`/`security` (closed account, USD no-rate, ROC only year) each "n/a" or a case
- [ ] Step 4: `acb.go` `acbYearRefusal`/wording (as `asOfRefusal`, `holdings.go:63-80`), `accounts.go:38-50` `securityRefusal` beside `categoryRefusal`, `result.go:24-35,87` logs; fault rows: bad year (`24`, `""`, `+2024`, `20245`, `0000`), future year (this year ok, next refused), unknown security (first in argv order, `""`), R-6 (1 and N accounts, ignored finding does not unblock), unreadable config -> `configRefusalLog("acb")` before the store, `currency` param refused by the schema, no `year` echo on stderr; year refused before config and store (class line)

### Build (B2)
- [ ] Step 5: `cap.go:9-16` extract the cap line from `capList`; `acb.go` `capEvents` over `doc.Securities[].Events`; `Test_acb_caps_events_at_500_across_securities` (500 whole, 501 cut, events spread over 2 securities, later security `events: []`, warning last, `security` narrows below the cap), `capList` callers unchanged
- [ ] Step 6: docs/pins: `report/sql_conventions.go:31-33` sentence 2 directly after sentence 1, re-wrapped; rewrite `sql_conventions_test.go:37-42` (drop `NotContains "acb"`, assert sentence 2); hand copies (`grep -rln 'cost_basis is the cost' cmd internal plugin` lists them) `internal/cli/sql_test.go:234` `Test_sql_help_describes_the_command_and_its_flags`, `cmd/quarry/run_shared_documents_test.go:397` `Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte`, `run_holdings_surfaces_test.go:~25-40` `Test_run_accounts_and_sql_help_carry_the_holdings_copy`; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`; `cli/mcp.go:40-41` + `cli/mcp_test.go:43-45` + `run_mcp_descriptions_test.go:286` Tools line; `plugin/skills/quarry/SKILL.md:98` §9 and §8 row (`:80-90`) + `run_skill_text_test.go:226,240`

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/mcp` reads right; doc comments on every new symbol

### Verify
- [ ] Step 8: full verification per `agent-briefs.md`; `.claude/scripts/spec-check.py phase4de-acb`; tick SCENARIO-19 in `specification.md` with its acceptance test last on the line; rewrite STATE.md (drop the S19 Left-unbuilt items, the `years[]` pin and conventions-test debts)

## Handoff
**Binding decisions:**
- MCP `acb` renders `acb.Cut()` and builds warnings from the UNCUT `acb` through the one `document.ACBWarnings` — a cut report fires slot 2 form 1 and drops warnings 4/6/7 (STATE trap).
- `acbAdjustmentsOf` and `classificationOf` are per delivery package; the parity case with an `[[acb.adjustment]]` is the pin that the mcp copy stays in step.
- Cap is on total events (500), never on sales or securities; `years[]` and `securities[]` headers always complete.
**Left unbuilt:** `dataQualityDescription` kinds (`tools.go:77-84`, pins `run_mcp_descriptions_test.go:~50-56`) — final pass; `finding.go:322` LIRA/401(k)/IRA — ruling 7; SCENARIO-21 reference check — orchestrator.
**Traps:** `fakeStore`/`stallingStore` embed a nil `report.Store`: any test reaching `InvestmentHistory` panics without the new stub. `accountRefusal`-style wrapping keeps caller text off stderr only if `refusalLine` maps the kind to a class line. Hand-copy conventions pins shift when sentence 2 re-wraps the paragraph.
