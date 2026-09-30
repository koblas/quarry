---
id: SCENARIO-19
status: open
---

# SCENARIO-19: status shows the open findings count (folds SCENARIO-20, status warns on a bad config)

Cadence: code-first (nothing on the mandatory test-first set: read-only, no write guard, no atomic adapter)
Acceptance test: `cmd/quarry/run_status_findings_test.go` `Test_run_status_shows_the_findings_line_with_open_and_ignored_counts`
Acceptance test (SCENARIO-20, folded): `cmd/quarry/run_status_findings_test.go` `Test_run_status_warns_on_a_bad_config_and_still_reports`
Narrow loop: `go test ./internal/report/ -run 'findingCounts|status' && go test ./internal/cli/ && go test ./cmd/quarry/ -run 'status|Status|read_commands'` (whole `internal/cli`: `-run` misses `Test_*_internal` names)
Mutation checks: `FindingCounts` passes `ignore` to `finding.Classify` → `Test_findingCounts_*ignored*` (report) and the acceptance test; refusal path passes a nil ignore list and `Ignored` nil → `Test_run_status_warns_on_a_bad_config_and_still_reports`; status prints no C3/W1 → `Test_run_status_stays_silent_on_unknown_keys_and_unmatched_ignore_ids`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`report`; `cli`, `cmd/quarry` tests). Sizing said 4: SCENARIO-11 already put `Store.Findings` on the `report.Store` port, `duckstore` implements it and `cmd/quarry/run.go:25-30` already guards `report.Store`, so the "port read + adapter" batch and the port-guard edit do not exist. No `internal/config` edit (see Handoff).

Inventory (survey of what `status` needs; nothing new on the port): `report.Store.Findings` (exists, `store.go:18`) -> reused; `report.Server.Status` (`report.go:50`) -> stays; `ConfigLoader` (`cli/run.go:32`, wired `cmd/quarry/run.go:118`, already in `cli.Env.LoadConfig`) -> new consumer `newStatusCommand`; `findingsPhrase` (`cli/render.go:103`) -> reused for the text; `finding.Classify` -> the only ignore matcher.

Contract. Text: `Findings` row after `Transfers` (`%-10s`), exactly `Findings  12 open, 4 ignored; run quarry findings to list them` / `Findings  none open` / `Findings  none open, 4 ignored`; counts thousands-grouped; never a `(M new)` or `fixed since the last sync` clause. `--json`: `"findings": {open, ignored, fixed, new, newly_fixed}` between `transfers` and `not_imported`, every other byte unchanged. Config load is best-effort, after the store read: never refuses, exit 0, no C3 and no W1. On ANY loader error stderr is `quarry: warning: cannot tell which findings you ignored: <err text minus "; fix the file and run the command again">; findings you ignored are counted as open`; counts then use a nil ignore list (ignored findings are in `open`, and in `new` when new), the text drops the ignored clause, JSON `ignored` is `null`. Store refusal (missing, R2 format, locked, interrupted) exits 1 with only its existing line; config is never read then. A v3/no-findings store is already the R2 refusal from `Status` (`run_read_refusals_test.go:102-118`): no new code, no new test.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_status_findings_test.go` `Test_run_status_shows_the_findings_line_with_open_and_ignored_counts` — sync a v9 fixture raising exactly 3 open + 1 ignored (`writeConfig` with `findings.ignore`; reuse `syncIgnoreFixture`'s shape in `run_findings_ignore_test.go:26-50`, add findings; open != ignored so a dropped ignore list reddens it); assert the whole `Findings  3 open, 1 ignored; run quarry findings to list them\n` row is last in stdout, stderr empty, exit 0. Red at the assertion (no row yet)
- [ ] Step 2: same file `Test_run_status_warns_on_a_bad_config_and_still_reports` — folded SCENARIO-20: same store, `snapshots.keep = 0`; stderr exactly `quarry: warning: cannot tell which findings you ignored: ` + `configShown` + `: snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open\n`, row `Findings  4 open; run quarry findings to list them`, exit 0. Red at its assertion

### Build
- [ ] Step 3: `internal/report/findings.go:53-79` `Findings` + new `(*Server).FindingCounts(ctx, ignore []string) (finding.Counts, error)` — lift the known-type filter / `finding.State` build (`:60-76`) into one unexported helper both use (no second ignore matcher); `FindingCounts` = `store.Findings` + `Classify(states, ignore).Counts`, fault via `s.readRefusal(ctx, "status", err)`. Trim `findingOrder` doc (`:126-128`) to 2 lines (STATE debt). Tests in new `internal/report/finding_counts_test.go` against `fakeStore` (`fakes_test.go:10`, `findings` field): counts of all five fields; an ignored+fixed finding counts fixed not ignored; an ignored new finding is not new; nil ignore -> `Ignored` 0, all open; unmatched id and unknown-type row change nothing; fault test: store error -> refusal/err as `Test_status_returns_the_store_fault` (`status_test.go:25`) and a cancelled ctx -> `status interrupted`
- [ ] Step 4: `internal/cli/render_status.go:19-31` `renderStatus`, `internal/cli/json_status.go:9-19,101-107` `statusDocument`/`newStatusDocument`/`renderStatusJSON` — new `statusFindings{counts finding.Counts; ignoreKnown bool}` parameter (update callers via `findReferences`: `status.go`, `render_status_internal_test.go:93-149`, `json_status_internal_test.go:75`); row = `findingsPhrase` with `New`/`NewlyFixed` zeroed and `carried` false; new `statusFindingsDocument{Open int; Ignored *int; Fixed, New, NewlyFixed int}` field `Findings` between `Transfers` and `NotImported`; `Ignored` nil unless `ignoreKnown`. Tests: the three ruled rows verbatim, `1,234 open` grouping, no new/fixed clause when `New`/`NewlyFixed` > 0, refusal form (`ignoreKnown` false) drops the clause, JSON key order/values and `"ignored": null`; fix the `Test_renderStatus`/`Test_renderStatusJSON` whole-output expectations (new row / key)
- [ ] Step 5: `internal/cli/status.go:11-41`, `internal/cli/root.go:28` `newStatusCommand(newReport, loadConfig, jsonOut)` — after `srv.Status` succeeds: `loadConfig("status")`; error -> `ignore=nil`, `ignoreKnown=false`, warning text = `strings.TrimSuffix(err.Error(), <cli const for "; fix the file and run the command again">)` in the ruled sentence; `srv.FindingCounts(ctx, ignore)` (fault -> `runtimeError`); `printConfigWarnings` only after the counts succeed, before stdout; put the unprefixed warning in `warnings[]` (see Handoff), nothing else (no `cfg.Warnings`, no W1). Long first paragraph: `…the dates its transactions cover, the checks sync ran when it built the store, and how many findings are open.` (re-wrap to 76 cols). `cmd/quarry` tests: fix `run_status_test.go:45-56` (new row, value derived from the fixture's findings), `:132-147` Long pin, `run_status_json_test.go:21-104` (new object); `run_config_test.go:205-231` `Test_run_read_commands_ignore_a_malformed_config`: DROP `status` from its table (it asserts silence, now wrong; status is covered below). New tests in `run_status_findings_test.go`: `--json` with the ignore list (all five values, `warnings: []`); a table over refusals — TOML syntax error, `snapshots.keep = 0`, `findings.ignore = "x"`, config.toml a directory (`cannot read …: is a directory`) — each exit 0, ruled line, `"ignored": null` and the warning in `warnings[]` under `--json`; `Test_run_status_stays_silent_on_unknown_keys_and_unmatched_ignore_ids` (`snapshot.keep = 3`, `ignore = ["uncategorized:payee-999"]`: empty stderr); `Test_run_status_refuses_a_missing_store_without_reading_the_config` (bad config, no store: only the `no store at …` line, exit 1); missing config file and valid config: no warning

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `FindingCounts`, `statusFindings`; `go doc ./internal/report` reads true

### Verify
- [ ] Step 7: full verification block + `spec-check.py phase2d-findings`; tick SCENARIO-19 with its acceptance test and SCENARIO-20 `— delivered by SCENARIO-19 — ` + its test (test reference last on the line); rewrite `STATE.md`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Config refused on `status` => ignore list is nil, NOT "unknown": would-be-ignored findings are in `open` (and `new`), `ignored` is null in JSON and absent from text — the spec's "counted as open", pinned by the folded acceptance test.
- `report.Server.FindingCounts(ctx, ignore)` is status's only read; it shares the known-type/`State` helper with `Findings` and calls `finding.Classify` — P2d-3 keeps one ignore matcher. Its refusals use the command word `status`.
- The refusal suffix is stripped in `cli` with a private const copy of `; fix the file and run the command again` (`config.fixIt` is private; touching `internal/config` would make this a two-feature-package scenario). The cmd-level refusal table runs the real loader, so a drifting copy reddens it.
- `status` warning text goes to stderr before stdout and, unprefixed, into `warnings[]` (mirrors `findings --json`'s config warnings, STATE `findings --json` entry). UNRULED by product-vision: the spec says "stderr" only. Flagged to the orchestrator.

**Left unbuilt**: `findings --csv`, the `--csv --json` line (22); sql `--csv` (21); types 24-28 unchanged.

**Traps**:
- `Test_run_read_commands_ignore_a_malformed_config` (`run_config_test.go:205`) asserts status silent on a bad config; it must lose `status` or it fails.
- `status --json` `new` is not gated by "history carried" (the store does not record it); after a first 2d sync `new == open`, unlike the sync text that hides `(M new)`.
- Status Long para 2 says "status reads only quarry's store"; spec changes only para 1, so it stays (config is read for the ignore list only).
- `cli` `fakeReportStore` embeds `report.Store`: a cli test that runs status through `Execute` needs its `Findings` set or it panics.
- `findingsPhrase(c, carried)` hides New only via `carried`; pass `New: 0, NewlyFixed: 0` AND `carried=false` for status.
