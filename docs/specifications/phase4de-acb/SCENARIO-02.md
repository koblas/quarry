---
id: SCENARIO-02
status: done
---

# SCENARIO-02: A malformed classification is refused

Cadence: code-first (masking and the "in both" refusal are not on the mandatory test-first set: no file write, no atomic adapter)
Acceptance test: `cmd/quarry/run_config_test.go` `Test_run_refuses_an_account_number_in_an_accounts_list_masked`
Narrow loop: `go test ./internal/platform/accountmask/ ./internal/config/ -run 'Mask|Account|Both|Load|Problem'` then `go test ./cmd/quarry/ -run 'masked|Masked'` and `go test ./internal/report/ -run 'Classif|ccount'`
Mutation checks: keep-last-four boundary in `accountmask.Mask` → `Test_Mask_keeps_the_last_four_digits`; `setting.masked` gate (findings.ignore stays unmasked) → `Test_load_leaves_a_findings_ignore_item_unmasked`; in-both guard in `(document).notInBoth` → `Test_load_refuses_an_id_listed_in_both_account_lists`
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (config) + 1 new platform package; report only loses one test and a doc line

## Existing-surface survey (no new port)

Echo sites in `internal/config/parse.go`, all reached through `file.badValue` (`refusal.go:46`) or `unknownKeys`; MCP gets the same text via `Problem`/`ProblemAbsolute` (`refusal.go:13,52`), so one masked string serves CLI and MCP:
- `d.got` (parse.go:319-333) called at 251 (list not a list), 310 (`accounts must be a table`), and also by keep/path/currency (must stay unmasked).
- `itemText` (parse.go:283-289) at 257 ("as item n") — `idList` is shared with `findings.ignore`, so masking is per-setting.
- `unknownKeys` (parse.go:352-367, two calls at 104-105) → `keyText` (374-390).
- `tree()` syntax error (parse.go:111-126), probed on go-toml v2.4.3 over 30 malformed inputs: first-line messages echo no value, only one char (`U+0039 '9'`, never masked: that text is not a value). Four shapes DO echo the key or table name, unmasked: `key N is already defined`, `key N should be a table, not a value`, `table N already exists`, `table N already exists as an array of tables` (N = last key part, e.g. `[accounts.12345678]` written twice).

Digit counting (ruled reading chosen): across the WHOLE string, ASCII or Unicode digits (`unicode.IsDigit`), all but the last four become `*`, every other rune kept. Both ruled examples hold ("12345678" → "****5678"; "RBC 2019 TFSA" has four digits, unchanged); per-run counting would leak "1234 5678 9012", PRD :265 says "last four digits". Exempt only the exact `^acct-[0-9]+$`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_config_test.go` `Test_run_refuses_an_account_number_in_an_accounts_list_masked` — `run` with `sync`, config `accounts.registered = [12345678]`; stdout empty, stderr `quarry: <configShown>: accounts.registered must hold only account ids in quotes, got ****5678 as item 1` + `configFix`, exit 1 (reuse `writeConfig`, `configShown`, `configFix`). Red: stderr shows `12345678`.

### Build
- [x] Step 2 (B1): new `internal/platform/accountmask/{doc.go,accountmask.go,accountmask_test.go}` `Mask(s string) string` — exported for S05's unmatched warning. Tests (own package): `Test_Mask_keeps_the_last_four_digits` rows 0/1/4 digits unchanged, 5 digits → `*2345`, 8 → `****5678`, 12 with separators (`1234 5678 9012` → `**** **** 9012`); exact `acct-12345678` and `acct-1` unchanged; each near-miss masked (`Acct-12345678`, `acct-12345678x`, ` acct-12345678`); ruled `RBC 2019 TFSA` unchanged and `RBC 2019 TFSA 5678` → `RBC **** TFSA 5678` (whole-string pin); non-ASCII digits and multibyte runes kept; `Mask(Mask(x)) == Mask(x)`; empty string.
- [x] Step 3 (B1): `parse.go:27-29,44-45,243-263,283-289,303-315,352-390,111-126` — config masking sites, one batch:
  - `setting` gains `masked bool`, set on `registeredSetting`/`nonRegisteredSetting`; `idList` (251, 257) and `lookup` (310) mask the got/item text when `s.masked`; mask the item text BEFORE appending ` as item n` (the `n` is a digit).
  - `unknownKeys` (363): for a key whose first part is `accounts`, mask each later part's rendered text (quote decision on the original part, as `keyPartText`); both `Warnings` and `WarningsAbsolute`.
  - `tree()` (123-125): mask the captured name in the four probed shapes only; any other message passes through untouched.
  - Tests in `accounts_test.go` (bad list: both lists × plain integer and quoted string → `"****5678"`; item 1, item 2 `********9012`; `accounts = 12345678`; `accounts = "RBC 2019 TFSA"` unchanged; unknown keys `accounts.12345678` → `accounts.****5678`, `accounts.acct-12345678` unchanged, `x12345678` outside accounts unchanged control, both warning lists) and `config_test.go:120-141` (four syntax shapes with `12345678`; an unmasked control `a` already pinned; `U+0039 '9'` message untouched). Control `Test_load_leaves_a_findings_ignore_item_unmasked` (`findings.ignore = [12345678]` → `12345678 as item 1`). `problem_test.go`: `ProblemAbsolute` of a masked refusal is masked, absolute path. Cross-command: `cmd/quarry/run_config_test.go` `Test_run_read_commands_refuse_a_masked_account_list` over `readCommandArgs()` (pattern at 274-286).
- [x] Step 4 (B2): `parse.go:86-94`, `config.go:46-52`, `report/classification.go:9-10`, `report/classification_test.go:80-87` — new `(document).notInBoth(registered, nonRegistered)` called after both `idList` calls, refusing via `badValue("an account must be in only one of accounts.registered and accounts.non-registered", "<quoted masked id> in both")` (id = first registered id, file order, also in non-registered; quote with `tomlstr.BasicString` after `accountmask.Mask`). Delete `Test_accounts_classify_an_id_listed_in_both_lists_as_registered` (S01 pin; config now refuses it) and reword the `Classification` doc line (an id in both is refused at load). Tests in `accounts_test.go`: `Test_load_refuses_an_id_listed_in_both_account_lists` rows: plain `"acct-3"`, masked `"****5678"` for `12345678`, two shared ids names the first in registered order, `""` in both; controls that load clean: same id twice in one list, `Acct-3` vs `acct-3` (case-sensitive), one list unset; ordering: bad `registered` refused before in-both; `Test_load_refuses_an_id_listed_in_both_account_lists_in_either_list_order`.

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/platform/accountmask` reads as the contract; `config.Load` doc (`config.go:46-52`) names the in-both refusal.

### Verify
- [x] Step 6: full verification block (`.claude/rules/agent-briefs.md`) + `.claude/scripts/spec-check.py phase4de-acb`; tick SCENARIO-02 with its acceptance test; rewrite STATE.md (registered-wins decision replaced by the refusal; `Left unbuilt` loses mask helper, in-both and syntax probe).

## Handoff

**Binding decisions:**
- `accountmask.Mask(s)` in `internal/platform/accountmask` is the one masking helper; S05's unmatched warning calls it on the raw id (`lists "<Mask(id)>"`), not a copy — PRD :265 says every echo.
- Digits counted across the whole string, last four kept; exempt only exact `acct-<digits>` — both ruled examples hold; per-run counting would leak spaced/dashed numbers.
- Masking is per-setting (`setting.masked`): `findings.ignore` echoes stay raw. An id in both lists is refused by `config.Load`, so `Classification.Of`'s registered-first order is unreachable from a config; it stays only as a defined tie-break for hand-built values.
- go-toml probe result: syntax errors never echo values, but echo duplicate/conflicting key and table NAMES (four shapes above); only those are masked, not whole messages (`U+0039 '9'` has "digits" that are not a value).

**Left unbuilt:** `accounts.* lists "<v>", which is not an account in quarry's store` warning (S03, folded into S05); `unclassified-account` finding, counts (S04/S05).

**Traps:**
- Masking `<item> as item 3` as one string counts the `3`: mask the item text first, then append.
- `got` is raw token text: a quoted `accounts = "acct-12345678"` is not exactly the id (quotes), so it prints `"acct-****5678"`; harmless, pin not needed. A masked non-bare key part renders bare (`accounts.****5678`) because quoting is decided on the original part. Neither has a ruled copy; if product-vision objects, change here only.
- Do not mask the whole `tree()` message or all of `got`: keep/path/currency refusals and `U+XXXX` text must stay as pinned.
- Interim S01 tests (`"a"`, `12`, `7`) stay: they now prove short values pass the mask unchanged.

## Phase report

Run V done; scenario complete, status done. Steps 5-6 ticked, SCENARIO-02 ticked in specification.md with its acceptance test, `spec-check.py phase4de-acb` OK, STATE.md rewritten.
- Build and lint: `go build ./...` ok, `golangci-lint run ./...` 0 issues. `go doc ./internal/platform/accountmask` reads as the contract; `config.Load` doc names the in-both refusal.
- Covered full suite (`-count=1 -coverpkg=./...`): rc=0. `uncovered-diff.py --profile ... baf4ed0`: 0 uncovered added lines in 0 runs. `go test -race` on accountmask, config, report: ok.
- `test-stats.py --base baf4ed0 --changed`: cmd/quarry 799 (+2), internal/config 76 (+10), internal/platform/accountmask 4 (+4), internal/report 436 (-1), TOTAL 1315 (+15); tempdir +1, disk +1 (cmd/quarry).
- No code changes in V. Mutations were done by B1/B2; V adds none.
- Next: checkpoint over `<start>` = baf4ed0, then SCENARIO-04.
