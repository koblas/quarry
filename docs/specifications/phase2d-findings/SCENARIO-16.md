---
id: SCENARIO-16
status: open
---

# SCENARIO-16: an id listed in findings.ignore is ignored

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_findings_ignore_test.go` `Test_run_findings_leaves_an_ignored_id_off_the_list_and_counts_it_in_the_footer`
Acceptance test (SCENARIO-17, folded): `cmd/quarry/run_findings_ignore_test.go` `Test_run_findings_warns_about_an_ignored_id_that_is_not_a_finding`
Narrow loop: `go test ./internal/finding/ ./internal/report/ ./internal/config/ ./internal/cli/ ./internal/store/... ./internal/importer/ ./internal/snapshot/ ./cmd/quarry/ -run 'ignore|classify|findings|replace|import|basic|sync'`
Mutation checks: none
Runs: A (1) | B1 (2-3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 4 batches; `report`, `snapshot`, `importer` (field copy only) — the architect's SPLIT was overruled by the orchestrator (a half would need an unapproved Gherkin line)

Sync seam chosen: per-finding facts travel UP (`store.Replaced`/`Result` → `snapshot.WithIgnore` recounts). Passing the list DOWN would change `Store.Replace` (128 call sites) or `Importer.Import` (135) — UP changes no port signature.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: new `cmd/quarry/run_findings_ignore_test.go` — both acceptance tests through `run()`, `writeConfig` (`run_config_test.go:53`), fixture shaped like `run_findings_test.go:19-45` (a duplicate pair + an uncategorized payee). 16: stdout is the uncategorized group only, footer `1 open finding; 1 ignored not shown (--status all)`, no hint line, stderr empty. 17: stderr is exactly `quarry: warning: `+`configShown`+`: findings.ignore lists "uncategorized:payee-999", which is not a finding in quarry's store; quarry skips it\n`, exit 0. No stubs — both compile today and fail at their assertions

### Build
- [ ] Step 2: `internal/finding/finding.go:95-115` new `State` (ID, Fixed, New, NewlyFixed) + `Classify(states, ignore)` → per-state `Status`es, `Counts`, `Unmatched` (ignore elements with no state, file order, one per element); `internal/report/findings.go:17-86` `Findings` rebuilt over `Classify` (known types only, groups from the statuses), `FindingsListing.Unmatched` added. Tests in `finding_test.go`: `Test_classify_counts_a_fixed_finding_that_is_ignored_as_fixed_and_newly_fixed`, `Test_classify_does_not_count_an_ignored_new_finding_as_new`, `Test_classify_lists_each_unmatched_element_in_file_order` (unmatched duplicate twice, `""`, matched duplicate absent), `Test_classify_with_no_ignore_list_counts_as_before_and_lists_nothing_unmatched`; `report/findings_test.go`: `Test_findings_lists_the_ignore_ids_that_match_no_finding` (incl. an id naming an unknown-type row → unmatched). Existing `report` count tests stay green unchanged
- [ ] Step 3: `internal/cli/findings.go:85-92` `FindingsRequest{Ignore: cfg.Ignore}`; W1 lines from `listing.Unmatched`, `<config>` = `homepath.Abbreviate(srv.Home(), cfg.Path)` (same form as C3), id via a new exported always-quoting `config.BasicString` extracted from `internal/config/parse.go:332-357` `keyPartText` (which keeps its bare-key branch and calls it); W1 printed after C3 via `printConfigWarnings` (`output.go:66-70`) and appended unprefixed after `cfg.Warnings` in `renderFindingsJSON`'s warnings. Tests: `config` `Test_basicString_quotes_and_escapes` (bare-looking `abc` quoted, `"`, `\`, `\n`, `\t`, U+0001, U+007F, non-ASCII kept); `cmd/quarry/run_findings_ignore_test.go` `Test_run_findings_json_lists_an_unmatched_ignore_id_after_the_config_warning` (stderr and `warnings[]`, abbreviated path), `Test_run_findings_shows_the_hint_when_findings_ignore_is_an_empty_list`, `Test_run_findings_hides_the_hint_when_findings_ignore_lists_only_unmatched_ids`, `Test_run_findings_counts_an_ignored_finding_that_is_fixed_as_fixed` (second sync without the pair; footer shows no ignored clause for it)
- [ ] Step 4: `internal/store/store.go:210-245` `Replaced`/`Result` gain `FindingStates []finding.State`; `internal/store/duckstore/findings.go:44-60,144-182` `mergeFindings`/`loadFindings` return states, `duckstore.go:298-331,394-409` `build`/`Replace` set `Findings` = `finding.Classify(states, nil).Counts` and `FindingStates`; `internal/importer/importer.go:134-138` copies the field. Tests: `duckstore/findings_test.go` `Test_replace_reports_each_findings_state_new_carried_reopened_and_newly_fixed`; `importer/import_runs_test.go:79` extend `Test_import_passes_the_carry_faults_through_to_the_result` or add `Test_import_passes_the_finding_states_through_to_the_result`. `findings_test.go:78,219` count tests must stay green unchanged — they now guard `Classify`
- [ ] Step 5: `internal/snapshot/snapshot.go:26-40,101-105` `WithIgnore(ids []string)`; `import.go:147-151` `importVerified` sets `result.Findings = finding.Classify(result.FindingStates, s.ignore).Counts` on a built store; `internal/cli/sync.go:95` passes `snapshot.WithIgnore(cfg.Ignore)` (covers `--from`); `internal/cli/render.go:103-120` `findingsPhrase` appends `, J ignored` after the fixed clause, before the tail. Tests: `snapshot` `Test_sync_and_import_counts_an_ignored_open_finding_as_ignored_not_open_or_new` + control without `WithIgnore`; `render_internal_test.go:299` `Test_findingsPhrase` rows `12 open (3 new), 2 fixed since the last sync, 4 ignored; run quarry findings to list them`, `none open, 4 ignored`, `none open, 2 fixed since the last sync, 4 ignored`, thousands-grouped ignored; `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_counts_an_ignored_finding_as_ignored_not_open` (text line + `store.findings.ignored`/`open`/`new` in `--json`, second sync so history is carried) and `Test_run_sync_says_nothing_about_an_ignored_id_that_is_not_a_finding` (stderr empty, `warnings[]` has no W1)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `State`, `Classify`, `BasicString`, `WithIgnore`, `FindingStates`, `FindingsListing.Unmatched`; fix `Findings`/`Result` docs that say counts come from detection only

### Verify
- [ ] Step 7: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-16 with its acceptance test, tick SCENARIO-17 `— delivered by SCENARIO-16 —` with its folded test

## Handoff

**Binding decisions:**
- `finding.Classify` is the ONLY place an ignore id is matched to a finding (P2d-3 via `StatusOf`); `report`, `duckstore` (nil list) and `snapshot` call it — 18's `--status` views and 19's `status` counts must use it, never a second map
- Sync ignore travels UP: duckstore never sees `findings.ignore`; `Result.Findings` from the store is ignore-free (`Ignored` 0) until `snapshot.importVerified` recounts — read counts only from the `Outcome`
- W1 is built in `cli` from `FindingsListing.Unmatched`: one line per unmatched element, file order (duplicates repeat, `""` prints `lists ""`), id via `config.BasicString`, `<config>` abbreviated like C3 on stderr AND in `warnings[]`, after C3. `sync`/`status` never print it
- Hint iff ≥1 open and `len(cfg.Ignore) == 0` — a list holding only unmatched ids still hides it

**Left unbuilt:**
- `status` Findings line / `status --json` `findings` and its best-effort config — 19 (must pass the ignore list to `Classify`)
- `--status`/`--type` filtering — 18; W1 stays computed over every known-type row, never the filtered view

**Traps:**
- `keyPartText` returns bare text for bare-key-shaped strings; W1 needs `BasicString` (always quoted), not `keyPartText`
- An ignore id naming a stored row whose type this binary does not know gets W1 (such rows are neither listed nor counted) — accepted reading
- `(M new)` only renders when history was carried: sync tests of ignored-new need a second sync
- Do not add a field to `config.Config` for the shown path — whole-struct `assert.Equal` tests break; abbreviate `cfg.Path` with `srv.Home()`

## Phase report
