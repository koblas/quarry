---
id: SCENARIO-04
status: done
---

# SCENARIO-04: Unclassified investment accounts are findings

Cadence: code-first — no bug fix, write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_findings_unclassified_test.go` `Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it`
Narrow loop: `go test ./internal/finding/ ./internal/report/... ./internal/store/duckstore/ ./internal/cli/ ./internal/mcp/ -run '(?i)finding|unclassif|csv|data_?quality'` then `go test ./cmd/quarry/ -run '(?i)findings|data_?quality|skill'`
Mutation checks: classification wiring `internal/cli/findings.go:91` (pass `report.Classification{}`) → acceptance test; wiring `internal/mcp/data_quality.go:31` (same) → `Test_run_mcp_data_quality_leaves_out_an_account_the_config_classifies`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`report`; `finding`, duckstore adapter, cli/mcp wiring do not count)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_findings_unclassified_test.go` (new) `Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it` — v9fixture with one BROKERAGENORMAL account (plus a chequing control), `syncBundle`; `quarry findings` stdout holds the ruled heading + group clause line and the text row; then `writeConfig` lists the id in `accounts.registered`, run again WITHOUT sync → row and group gone. One unclassified account only (see Traps: example order)
- [x] Step 2: `internal/finding/finding.go:15-33` `UnclassifiedAccount` const appended last in `Types()` + empty `fixes` entry; `internal/report/findings.go:20-27` `FindingsRequest.Classification` field — stubs so it compiles; must fail at the stdout assertion

### Build
- [x] Step 3: `internal/store/store.go:603-607` `FindingList.Accounts []Account`; `store.go:625-642` `FindingItem.AccountType` (unclassified-account item only); `internal/store/duckstore/findings_read.go:37-91` `(*Store).Findings` reads accounts (id, name, type, currency, closed, active) in the SAME open as the findings query — tests in `findings_read_test.go`: accounts read with closed/active; fault test: second query fails (drop a column only it reads, e.g. `accounts.type`) → `*store.OpenError`; update `internal/report/store.go:35` doc
- [x] Step 4: `internal/finding/finding.go:271-312` `fixes[UnclassifiedAccount]` ruled Heading / GroupClause / Sentence verbatim + per-type read-time predicate (e.g. `Type.ReadTime()`); `finding_test.go:10-13` re-pin to nine types, `finding_test.go:~204` fix table gains the verbatim row (all three fields, pins the JSON `fix` sentence); doc.go:1 + finding.go:15 "eight" → nine. New `internal/report/readtime.go` `readTimeFindings(list store.FindingList, c Classification) store.FindingList` (chokepoint) + `unclassifiedFindings` using `Classification.Unclassified`: id `finding.ID(UnclassifiedAccount, acct.ID)`, one item (account id/name/type/currency/closed), zero FirstFoundAt, never New/Fixed. Call it at `report/findings.go:54` BEFORE `knownFindings`/`Classify` (:60-69); `findingOrder` arm at `findings.go:140-177` (name case-insensitive, then id). Tests (`internal/report/readtime_test.go`, fake store with `Accounts`): brokerage + retirement unlisted → finding; listed registered → none; listed non-registered → none; non-investment unlisted → none; closed / not-in-reports / linked-tracking / no-transactions unlisted → finding; order rows: B-then-A and A-then-B names, case fold (`alpha` vs `Beta`), same name → id tiebreak; ignore id of an unclassified account → `ignored`, not `Unmatched`; ignore id of a classified account → in `Unmatched`; `--status fixed` lists none; `Counts.New` excludes them; `--type unclassified-account` counts only them; add the type's row to any per-type ordering table in `report/findings_test.go`
- [x] Step 5: `internal/report/document/findings.go:24,68-88` `FindingEntry.FirstFoundAt` → `*string`, null when the type is read-time (per-type predicate, NOT `IsZero` — cli fakes build findings without FirstFoundAt); sole production reader is :80; arm beside :74 sets `account_id`/`account`/`currency` only (date, amount, payee… stay null; AccountType never serialized). Tests `document/findings_test.go`: item key set + nulls, first_found_at null for this type and a timestamp for a stored type. `internal/cli/csv_findings.go` unchanged — pin in `internal/cli/findings_csv_test.go`: read back with `encoding/csv`, row count, `account`+`currency` filled, other item cells empty, `finding_id` = id
- [x] Step 6: `internal/cli/render_findings.go:120-141` `liveFindingLines` case + new `unclassifiedRows` (pattern `unusedCategoryRows` :335-348): id padded to widest, `escapeCell(name)` padded, `type, currency`, `, closed`, `ignoredMarker`. Tests in new `internal/cli/render_findings_unclassified_internal_test.go` (sibling of `render_findings_unused_internal_test.go:26`): two rows padded, escaped name, closed suffix, ignored marker under `--status all`; `internal/cli/findings_test.go:206` accepts-every-type table gains the row. `internal/cli/findings.go:91` passes `report.Classification{Registered: cfg.Registered, NonRegistered: cfg.NonRegistered}`; `internal/mcp/data_quality.go:31` same + `cmd/quarry/run_mcp_data_quality_test.go` `Test_run_mcp_data_quality_leaves_out_an_account_the_config_classifies` (control: unlisted sibling account listed). Copy: Long `findings.go:24-61` — new table row (realign every row to the 20-rune name column, re-wrap continuations at the existing ≤75-column margin), after-table paragraph + TOML example verbatim; `--type` help :119 gains `unclassified-account`; `--csv` help :120 Part A form. Re-pin `internal/cli/findings_test.go:43-81,113-141`. `plugin/skills/quarry/references/findings.md:3` first line + bullet after :22 verbatim — pin: add the ruled bullet body and `which accounts quarry needs classified` to the findings.md phrases row `cmd/quarry/run_skill_references_test.go:~79` (the helper pins only the bullet prefix)
- [x] Step 7: fixture re-pins per the Fixture rule (Handoff) — run `go test ./cmd/quarry/ ./internal/mcp/` and fix what fails; candidates: `cmd/quarry/run_config_test.go`, `run_investment_cash_test.go`, `run_shared_documents_test.go`, `run_mcp_descriptions_test.go` (:212 enum literal), `run_findings_usage_test.go`; JSON readers of `first_found_at` (`run_findings_json_test.go`, `run_findings_filters_test.go`, `run_shared_documents_test.go`, `run_investment_cash_test.go`) decode null into `string` as `""` silently — check any with an investment account (`--type must be …` derives from `Types()`); MCP enum derives from `Types()` — re-pin only

### Sweep
- [x] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (exhaustive switches on `finding.Type`); doc comments on `FindingList`, `FindingItem.AccountType`, `FindingsRequest`, `readTimeFindings`, `(*duckstore.Store).Findings`

### Verify
- [x] Step 9: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-04 with its acceptance test; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Read-time chokepoint is `report.readTimeFindings(store.FindingList, Classification)` in `internal/report/readtime.go`, merged before `finding.Classify` — S05 calls it from `CountFindings` (build a `FindingList` from `store.Status` findings + accounts); 13b adds its detector inside it and its input as a new `FindingList` field, not a new parameter
- Accounts ride on `store.FindingList.Accounts`, read in the same open as findings — report.Store is at its 10-method cap
- Read-time status is per type (`finding` predicate), not "FirstFoundAt is zero": JSON `first_found_at` null, never New/NewlyFixed/Fixed. 13b's type sets the same predicate
- Fixture rule: no default classifying config, no edit to `syncBundle` or any shared helper (fixtures are per-test v9fixture builders; S01 re-pinned accounts the same way). A test whose subject is the findings list/counts re-pins (adds the row/count); any other test narrows with `--type` or adds a local `writeConfig` with `accounts.non-registered`. Binds S05's 12 status/sync count pins
- Long type table realigned to a 20-rune name column (descriptions at col 24) — shared-format choice; orchestrator may want a product-vision nod before B2. `shares-without-cost` (19) fits

**Deviation from brief:** `mcp/data_quality.go:31` wired here — the detector runs inside `Server.Findings`, so an unwired caller would list every investment account regardless of config. S05: data_quality listing and its counts are done; sync_status/status/sync remain

**Left unbuilt:**
- status/sync/MCP sync_status counts, `CountFindings` through `readTimeFindings` — S05; unknown-id warning — S03 in S05
- findings.md "Classifying accounts" section (bullet link dangles by design) — S17
- `plugin/skills/quarry/references/findings.md:42` "one row per transaction, split, payee or category" — unruled copy gap (mirrors `--csv` help); left as is, orchestrator to rule (13b too)

**Traps:**
- Spec example (Surface & Copy, Part A) shows `Questrade TFSA` acct-12 before `Old RRSP` acct-31 — id order, contradicting the ruled name-then-id order. Code follows the RULE; never copy the example as a two-row literal
- Until S05, `quarry findings` counts include these findings while `status`/`sync` do not — expected, owned by S05
- `NewFindingItem` returns early with no transaction/split (`document/findings.go:101`); the type arm must live in `NewFindingEntry`
- duckstore sync-time detectors must never emit `unclassified-account`; add no detector-table entry there

## Phase report

Run V (steps 8-9) done: `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite rc 0; `uncovered-diff.py d012715` 0 uncovered added lines, 2 declared unreachable (`internal/cli/render_findings.go:378 accountItem`, `internal/report/findings.go:226 accountOf`; reasons hold, same shape as `categoryOf`); `go test -race` on report, document, finding, cli, mcp, duckstore green; `spec-check.py phase4de-acb` OK. Ticked in specification.md; STATE.md rewritten.
test-stats --base d012715 --changed: cmd/quarry 801 (+2), internal/cli 522 (+6), internal/finding 16 (+1), internal/report 446 (+10), internal/report/document 166 (+2), internal/store/duckstore 700 (+3); TOTAL 2651 (+24), tempdir 736 (+2), disk 647 (+2).

Earlier: Run B2 (steps 6-7) done. Acceptance GREEN.

Changed:
- `internal/cli/render_findings.go` `liveFindingLines` case + `unclassifiedRows` + `accountItem` (empty-items branch marked `// unreachable`); `internal/cli/findings.go` Long (table realigned to 20-rune name column, wrapped at 51 cols, greedy re-wrap reproduces the old table at the old width), after-table paragraph + TOML example after the "duplicate and unlinked-transfer" note, `--type`/`--csv` help, `report.Classification` passed in `FindingsRequest`; `internal/mcp/data_quality.go` same wiring.
- Tests: `internal/cli/render_findings_unclassified_internal_test.go` (new, 5), `internal/cli/findings_test.go` re-pins (Long, flag help, bad-type usage, accepts-every-type row), `cmd/quarry/run_mcp_data_quality_test.go` `Test_run_mcp_data_quality_leaves_out_an_account_the_config_classifies`, `run_skill_references_test.go` findings.md phrases (+bullet body, first-line phrase).
- Fixture re-pins: `run_findings_usage_test.go`, `run_read_usage_test.go`, `run_mcp_descriptions_test.go:212` (type literals); `run_investment_cash_test.go` (`...keeps_an_ignored_uncategorized_finding_ignored`: local `accounts.non-registered` in its existing config write). No other cmd/quarry test needed changing; `first_found_at` JSON goldens hold no investment account.
- `plugin/skills/quarry/references/findings.md`: first line + bullet verbatim.

Mutations (both reverted, diff clean):
- `internal/cli/findings.go` `Classification: report.Classification{Registered..}` -> `report.Classification{}`: `Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it` red ("unclassified-account" still listed after config classifies it).
- `internal/mcp/data_quality.go` same -> `Test_run_mcp_data_quality_leaves_out_an_account_the_config_classifies` red (`Len` 1 vs 2 findings).

`report.Classification{Registered: cfg.Registered, NonRegistered: cfg.NonRegistered}` is now spelled at three sites (accounts.go, findings.go, data_quality.go) - S05 adds more; consider hoisting then.
Open (unruled): findings.md "one row per transaction, split, payee or category" (line ~42) not updated to the `--csv` help form.
