---
id: SCENARIO-15
status: done
---

# SCENARIO-15: A malformed findings.ignore is refused before Quicken is touched

Cadence: code-first (config parsing only; no write-safety guard, no atomic adapter)
Acceptance test: `cmd/quarry/run_findings_ignore_config_test.go` `Test_run_sync_refuses_a_findings_ignore_that_is_not_a_list_before_taking_a_snapshot`
Narrow loop: `go test ./internal/config/` and `go test ./cmd/quarry/ -run 'findings_ignore|config'`
Mutation checks: none (code-first)
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`internal/config`); `cmd/quarry` holds only the acceptance test

Inputs read: STATE.md (no config decision beyond "Config ignore field, C6/C6e — 15"), 2c spec C2a/C2l/C2m/C2t rows, `go doc ./internal/config`. Consumers of `config.Config`: `cmd/quarry/run.go:122-125` (`newConfigLoader`) and tests only; no help/doc copy lists the known keys (grep of `*.go` and `docs/` outside specifications), so the P2c-1 amendment is the `knownKeys` var.

## Pinned strings (`<config>` = `~/Library/Application Support/quarry/config.toml`, each line = `quarry: ` + text + `; fix the file and run the command again`)
- C6: `<config>: findings.ignore must be a list of finding ids in quotes, such as ["duplicate:txn-4410+txn-4412"], got <raw>` — raw = `"duplicate:txn-4410+txn-4412"`, `12`, `true`; header forms `got a table` / `got a list of tables`; multi-line raw collapsed (whitespace runs → one space, trimmed)
- C6e: `<config>: findings.ignore must hold only finding ids in quotes, got 12 as item 3` — index from 1, raw element collapsed, lowest offending index
- C2t+: `<config>: findings must be a table, such as findings.ignore = ["duplicate:txn-4410+txn-4412"], got 3` (`got "x"`, `got a list of tables` for `[[findings]]`)
- C3: `<config>: unknown key findings.ignored; quarry ignores it` (warning, exit 0); `findings` and `findings.ignore` never warned

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_findings_ignore_config_test.go` `Test_run_sync_refuses_a_findings_ignore_that_is_not_a_list_before_taking_a_snapshot` — mirror `run_config_test.go:68-88` (good sync first; then `findings.ignore = "duplicate:txn-1+txn-2"`; second sync leaves snapshot count and store digest unchanged, stdout empty, exit 1, stderr = exact C6 line with `got "duplicate:txn-1+txn-2"`). Reuses `writeConfig`, `fileDigest`, `countFilesWithSuffix`, `configShown`, `configFix` from `run_config_test.go`. Red today: the key is unknown, so sync succeeds with a C3 warning. No stubs needed.

### Build
- [x] Step 2: `internal/config/config.go:19-29` `Config.Ignore []string` + `parse.go:30-33` `ignoreSetting` (table `findings`, name `ignore`, example `findings.ignore = ["duplicate:txn-4410+txn-4412"]`) + `parse.go:47-69` `parse` (call `doc.ignore()` after `quickenPath`) + new `(document).ignore()` beside `quickenPath` (`:185`) via `lookup` (`:202`) and `got` (`:218`) — happy path and C6 and C2t+. `Ignore` holds elements as written, file order, duplicates/`""`/unknown prefixes kept (W1 in 17 needs them), nil when absent (existing `assert.Equal(config.Config{Path, Keep})` tests at `config_test.go:63,83,170` need it). Tests in `config_test.go`: `Test_load_reads_findings_ignore_in_file_order_keeping_duplicates_and_empty_ids`; absent and `[]` → empty; `Test_load_refuses_a_findings_ignore_that_is_not_a_list` (string, int, bool, float, date, literal string; `[findings.ignore]`, dotted `findings.ignore.x = 1` → `got a table`; `[[findings.ignore]]` → `got a list of tables`, never C6e; multi-line inline table raw collapsed); `Test_load_refuses_findings_as_a_plain_value` (`findings = 3`, `"x"`, multi-line array, `[[findings]]`); `Test_load_reads_findings_ignore_from_an_inline_table` (`findings = { ignore = ["a"] }`); case variant `Findings.Ignore` warns and is not read; `Test_load_checks_findings_ignore_after_quicken_path` (snapshots.keep → quicken.path → findings order).
- [x] Step 3: `parse.go:91-98` `entry` + `:121-131` `appendKeyValue` + `(document).ignore()` — C6e: capture each array element's raw text (`entry` gains items). Elements of the TOML array only, counting from 1. Tests `Test_load_refuses_a_findings_ignore_element_that_is_not_a_string`: item 1 and item 3 of `["a", "b", 12]`, bool, float, date, nested array `["a", 1]`, inline table `{ a = 1 }` split over lines (collapsed), two bad items → first reported, comment lines between elements do not shift the index, trailing comma.
- [x] Step 4: `parse.go:235` `knownKeys` (add `{"findings"}`, `{"findings","ignore"}` via `ignoreSetting.key()`) + `config.go:31-35` `Load` doc (known keys now three). Tests: `Test_load_warns_about_an_unknown_key_under_findings` (`findings.ignored`, `[findings] other = 1` in file order with the existing unknown-key cases, exact line); `Test_load_has_no_warnings_when_every_key_is_known` extended with `findings.ignore`.

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Config.Ignore`, `ignore()`; keep `parse()` doc line in step with the new order.

### Verify
- [x] Step 6: full verification + `.claude/scripts/spec-check.py phase2d-findings` → tick SCENARIO-15 with its acceptance test; rewrite STATE.md (drop "Config ignore field, C6/C6e — 15" from Left unbuilt; add the `Config.Ignore` decision).

## Handoff

**Binding decisions**:
- `config.Config.Ignore []string` is `findings.ignore` exactly as written: file order, duplicates, `""` and unknown prefixes kept, nil when absent — 16 fills `FindingsRequest.Ignore` from it; W1 (17) reads the same list and needs file order and every element for one line per element
- Validation order is `snapshots.keep`, `quicken.path`, `findings.ignore`, then unknown keys — extends the 2c keep-before-path order; first refusal wins
- Only the first non-string element is reported (lowest index from 1)
- `findings` is a known table key, like `snapshots` and `quicken`; C2t+ reuses `lookup` with `ignoreSetting.example`

**Left unbuilt**:
- `FindingsRequest.Ignore` filled from `cfg.Ignore`, `Counts.Ignored`, `J ignored` clause — 16
- W1 warning, `status` best-effort loading — 17, 19/20

**Traps**:
- go-toml `unstable` `Array` nodes have no `Raw` range and `InlineTable` `Raw` is only its opening byte, so a compound element's raw text cannot be read from its node; scalars have `Raw`. Array children also include `Comment` nodes: skip them when counting items
- `[[findings.ignore]]` decodes to a `[]any` of maps, which looks like an array of non-strings: the entry kind (`ArrayTable`) must route it to C6 `got a list of tables`, not C6e
- `Config` comparisons in existing tests use `assert.Equal` on the whole struct: a non-nil empty `Ignore` for an absent key breaks them

## Phase report

Runs A, B1 and V done; all steps ticked. Run V: Sweep clean (`go build`, `golangci-lint` 0 issues, no doc-comment changes needed), full suite green, 0 uncovered added lines, spec ticked, STATE.md rewritten.
- `internal/config/config.go`: `Config.Ignore []string`, `Load` doc names the three known keys.
- `internal/config/parse.go`: `ignoreExample`, `ignoreSetting`, `parse()` calls `doc.ignore()` after `quickenPath`, `(document).ignore()` (C6, C6e, C2t+ via `lookup`), `(document).written(key)` (KeyValue entry only, so `[[findings.ignore]]` and dotted/header forms fall to C6), `knownKeys` gains `findings` and `findings.ignore`.
- `internal/config/items.go` (new): `arrayItems(text)` splits an array's written text into top-level items (skips comments, honours basic/literal/multi-line strings and nesting); `ignore()` reports the first item not starting with a quote. Deviation from plan step 3: `entry` did not gain an items field, since a node cannot give a compound item's text; items are split from `entry.value` instead. Non-string detection is textual, `Ignore` values come from the decoded tree.
- Tests: new `internal/config/ignore_test.go` (11 tests/tables); `config_test.go` `Test_load_has_no_warnings_when_every_key_is_known` extended with `findings.ignore`.
- Green: `go test ./internal/config/` (99.5% stmt; only pre-existing `tree` unreachable branch is below 100), `go test ./cmd/quarry/ -run 'findings_ignore|config'` (acceptance test now green, unedited), `golangci-lint run ./internal/config/...` 0 issues. Full suite, coverage gate, test-stats not run (V).
- V notes: `Config.Ignore` added to a struct compared with `assert.Equal` whole-struct in existing tests: nil when absent keeps them green. No mutation checks owed (code-first).
