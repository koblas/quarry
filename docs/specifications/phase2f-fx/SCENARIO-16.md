---
id: SCENARIO-16
status: open
---

# SCENARIO-16: A bad reporting.currency value refuses the read commands

Cadence: code-first (no mandatory test-first item: no write path, no guard on user files, no atomic adapter)
Acceptance test: `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_a_bad_reporting_currency`
Acceptance test (SCENARIO-15, folded): `cmd/quarry/run_read_usage_test.go` `Test_run_read_commands_refuse_a_bad_currency_flag`
Narrow loop: `go test ./internal/platform/money/ ./internal/config/ ./internal/cli/ ./cmd/quarry/ -run '(?i)currency|config|help|usage|refus'`
Mutation checks: config-skip when `--currency` is Changed in `resolve` → `Test_read_commands_read_the_config_once_and_only_without_the_currency_flag`; Changed-guard on flag validation → `Test_currency_flag_is_checked_only_when_given` (`--currency=` arm)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (config) + platform/money + cli + root wiring

Batch order is money+config → binder → resolver → cmd rows (the sizing's 15-first order is reversed because flag validation needs `money.ParseCurrency`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_read_refusals_test.go` (new test after :81) `Test_run_read_commands_refuse_a_bad_reporting_currency`. HOME has a config and no store. Rows: `reporting.currency = "EUR"` × spend, cashflow, recurring, anomalies, accounts; `""`, `12` and `true` × spend. Each row: exit 1, stdout empty, stderr `quarry: <configShown>: reporting.currency must be CAD, USD or native, got <as written>` + configFix. The sql row gets the no-store refusal, which shows sql ignores config. Red today at stderr, because the no-store refusal wins.
- [x] Step 2: `cmd/quarry/run_read_usage_test.go` (new test after :70) `Test_run_read_commands_refuse_a_bad_currency_flag` — `--currency EUR` × the five; exit 2, stdout empty, stderr `quarry: --currency must be CAD, USD or native\n`. Red today at stderr (cobra's unknown flag). No stubs: both compile now.

### Build
- [x] Step 3: money + config.
  - `internal/platform/money/money.go:8-17`: add `ParseCurrency` (case-insensitive, no trim) and `(Currency).String` (`CAD`/`USD`/`native`), tested in `money_test.go`.
    - Accepted arms: `CAD`, `cad`, `Cad`, `USD`, `usd`, `native`, `NATIVE`.
    - Refused: `""`, `EUR`, `" CAD"`, `"CAD "`, `nativ`.
    - `String` of each constant, plus an out-of-range value.
  - `internal/config/config.go:19-33`: add `Config.Currency`. The missing-file default at `:42-44` is CAD.
  - `parse.go`:
    - `:84-88`: add `reportingSetting`.
    - `:102-130`: `document.currency()` runs after `ignore`.
    - `:342-349`: add to `knownKeys`.
  - `config_test.go`:
    - `:57-87`: defaults gain `Currency: money.CAD` (missing, empty, comment only), plus a row for a file with no reporting key.
    - Reads: `"usd"` → USD, dotted form vs `[reporting]` table, `"native"`.
    - Refusals with got as written: `"EUR"`, `"eur"`, `""`, `12`, `true`.
    - `reporting = 1` row in the `:384` shape table.
    - Order row: bad ignore + bad currency → ignore refusal (precedent `:365`).
    - Warnings: `reporting.colour` warns; `Reporting.Currency` warns and is ignored (precedent `:300`/`:342`).
    - `ProblemAbsolute` form.
- [x] Step 4: binder. New `internal/cli/currency.go` `currencyFlag`.
  - `bind` takes a `code` placeholder, a `""` default and two help consts (four reports / accounts, ruled copy).
  - Validation runs in Args after `noArgs` (`errors.go:15-20`), only when `Changed("currency")`. It returns `UsageError{"--currency must be CAD, USD or native"}`.
  - Bind and Args in: `spend.go:46,74-75`, `cashflow.go:63,92`, `recurring.go:47,71`, `anomalies.go:41,65`, `accounts.go:18,44`.
  - Tests in `report_help_test.go`:
    - new `Test_each_report_shows_the_currency_flag` near `:136`: 5 rows, `(?m)--currency code +<help>$`, no `(default`;
    - cashflow `:107` gains a `--currency` row.
  - Tests in `currency_test.go` (cli_test), including `Test_currency_flag_is_checked_only_when_given`:
    - flag absent → exit 0, and `--currency=` → exit 2 (pair differing in one variable);
    - `--currency usd` accepted;
    - `--currency EUR --by bogus` → the currency message;
    - `extra --currency EUR` → the noArgs message.
- [ ] Step 5: resolver + wiring.
  - `currency.go` `resolve(cmd, loadConfig)` returns the currency, config warnings (`~` and absolute) and an error:
    - Changed → the parsed flag, loader never called;
    - otherwise `loadConfig(cmd.Name())`; an error → `&runtimeError`;
    - otherwise `cfg.Currency`.
  - Call `resolve` in each RunE after its own flag checks and before `openReport`: `spend.go:53-58`, `cashflow.go:64-75`, `recurring.go:48-55`, `anomalies.go:43-49`, `accounts.go:19-20`. The value is discarded with `_` in all five.
  - `printConfigWarnings(shown)` first (`output.go:67-71`). The JSON renderer gets absolute + own; `emitReport`/`emit` gets own only (precedent `findings.go:77-96`).
  - The five constructors gain `ConfigLoader`; `root.go:30-34` passes `env.LoadConfig`.
  - Every cli test `Env` for the five gains a loader returning `config.Config{Currency: money.CAD}`: `spend_test.go:22,87,132`, `spend_window_test.go:106`, `cashflow_test.go:31,104,119,178`, `recurring_test.go:20,99,114`, `anomalies_test.go:20,42,56`, `accounts_test.go:36,136,160`, `report_clock_test.go:29`.
  - Tests:
    - `Test_read_commands_read_the_config_once_and_only_without_the_currency_flag`: 5 × {flag absent → 1 call; flag given with an erroring loader → 0 calls, exit 0};
    - unknown-key warnings × 5 × {text: `~` line on stderr before the command's own warning; `--json`: `warnings[0]` absolute, stderr `~`};
    - loader error → exit 1 with its text.
    - Internal `currency_internal_test.go` covers `resolve`, because the resolved value cannot be observed through `cli.Run` until 08: the flag beats config, the config's USD applies when the flag is absent, and a CAD default applies when the config leaves it unset.
- [ ] Step 6: cmd all-commands rows.
  - `run_read_usage_test.go:30-60` rows: `spend --currency=`, `spend --currency EUR --by bogus`. New `Test_run_spend_refuses_a_bad_currency_flag_before_reading_a_bad_config` (exit 2).
  - `run_usage_test.go:199-218`: bare `--currency` × 5 → `quarry: flag needs an argument: --currency; Run 'quarry <cmd> --help' for usage.`
  - Split `run_config_test.go:209-252` into:
    - `Test_run_read_commands_refuse_a_malformed_config` (5, exit 1, `cannot read …: line 1: …`);
    - `…_ignore_a_malformed_config_when_given_a_currency` (5 with `--currency CAD`, output unchanged, stderr empty);
    - `Test_run_sql_ignores_a_malformed_config`;
    - a `spend --since bogus` + bad config row → exit 2.
  - New `Test_run_spend_warns_about_an_unknown_config_key_and_json_names_it_absolutely` (precedent `:267`, `:393`).
  - n/a, with reasons:
    - `run_read_refusals_test.go:46`/`:163`: a missing config gives the defaults, so the rows are unchanged.
    - Home-refusal pins `run_spend_refusals_test.go:72-83` and `run_accounts_test.go:65` keep the same text (the loader resolves home with the same command name). They must stay green.
    - `json_*` key-set pins: no key is added; `currency` comes in 08+.
    - `run_cashflow_invariant_test.go:76`: belongs to 10.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Add doc comments on `ParseCurrency`, `String`, `Config.Currency`, `currencyFlag` and `resolve`. Leave the Long texts alone (08/10/17-19 own them).

### Verify
- [ ] Step 8: run full verification and `spec-check.py phase2f-fx`. Tick SCENARIO-16 with its acceptance test, and SCENARIO-15 with "delivered by SCENARIO-16" before its test.

## Handoff

**Binding decisions:**
- `money.ParseCurrency(string) (Currency, bool)` + `Currency.String()` (`CAD`/`USD`/`native`) is the one parser for the flag and the config. It is case-insensitive with no trim. 08's JSON `currency` uses `String`.
- `config.Config.Currency` is CAD when the file is missing or empty, or the key is unset.
- Order: Args (noArgs, then `--currency` when Changed) → RunE's own flag checks (`--by`, window) → config, only when the flag is absent → store. Exit 2 always beats a config or store refusal.
- `resolve` returns the value, and all five RunEs discard it with `_`. `report` is untouched here. 08 adds `report.SpendRequest.Currency` and threads it in; 10 does CashFlow; 17-19 do the rest.
- Config warnings go to stderr in `~` form before output. JSON `warnings[]` lists the absolute config warnings first, then the command's own.
- Whole-file validation, as for existing keys: a bad `reporting.currency` also refuses sync, findings and snapshots, and turns status into its `cannot tell which findings you ignored: …` warning.

**Left unbuilt:** `report.SpendRequest.Currency`/`CashFlowRequest.Currency` (08/10); request fields for recurring, anomalies and accounts (17-19); top-level `currency` JSON key and captions (08+); Long-help edits (08/10/17/18/19).

**Traps:**
- `money.Currency`'s zero value is `Native`. A `config.Config{}` (or a fake loader returning one) silently puts the five commands in native mode.
- Validate only when Changed. The `""` default would otherwise refuse every bare command. A non-empty default adds cobra's `(default "CAD")` to the help and breaks the help pin.
- Two warning slices: passing the concatenated list to `emitReport` prints config warnings twice, once in absolute form.
- A nil `LoadConfig` panics. Every `Env` that runs one of the five needs a loader.
- `got` is as written: `"eur"` refuses with `got "eur"`, never upper-cased.

## Phase report

Run B1 (steps 3-4) done; steps 5-8 open. Narrow loop green: money, config, cli. cmd: `Test_run_read_commands_refuse_a_bad_currency_flag` (SCENARIO-15) is green; `Test_run_read_commands_refuse_a_bad_reporting_currency` stays red (no resolver yet, B2 turns it green).

- `internal/platform/money/money.go`: `ParseCurrency` (strings.ToLower, no trim; the long-s fold is refused) and `Currency.String` (out of range reads `native`), with a `//nolint:exhaustive` on its switch.
- `internal/config/config.go`, `parse.go`: `Config.Currency` (CAD when the file is missing or the key unset), `reportingSetting`, `document.currency()` after `ignore`, and known keys `reporting` and `reporting.currency`. The table-shape refusal comes from `lookup` with the ruled copy, `got` as written.
- `internal/cli/currency.go`: `currencyFlag{code}` with `bind(cmd, help)` and `args` (noArgs, then validation only when Changed). Consts `reportCurrencyHelp` and `accountsCurrencyHelp` hold the ruled help. Bound and set as `Args: currency.args` in spend, cashflow, recurring, anomalies and accounts. `resolve` and the ConfigLoader are NOT built (B2).
- Tests: `internal/config/currency_test.go` (new), rows in `problem_test.go`, defaults in `config_test.go`, `money_test.go` (3 tests), `internal/cli/currency_test.go` (new; its Env already carries a LoadConfig), `report_help_test.go` (new `Test_each_report_shows_the_currency_flag_without_a_cobra_default`, cashflow `--currency` row).
- Ruled-behaviour pins owned by B1 and green now (all cmd/quarry):
  - sync: rows `reporting.currency = "EUR"` and `reporting = "CAD"` in `Test_run_sync_refuses_a_bad_config_value_with_the_ruled_copy`.
  - snapshots: a row in `Test_run_snapshots_refuses_a_bad_config_value_with_nothing_on_stdout`.
  - snapshots prune: a row in `Test_run_snapshots_prune_refuses_a_bad_config_value_with_nothing_deleted`.
  - findings: `Test_run_findings_refuses_a_bad_reporting_currency_before_looking_for_a_store`.
  - status: two rows in `statusConfigRefusals()`, so the text and `--json` tests both pin the P2d-10 warning, exit 0 and `"ignored": null`.
- B2 still owns: the five read-command cmd rows in step 6 (bad currency flag, `--currency=`, bare flag, the `run_config_test.go:209` split, the unknown-key warning test). `--json` absolute-path pins for sync, snapshots and findings are not added (existing sync `--from` json test covers sync only).
- Changed an existing pin: `cmd/quarry/run_accounts_test.go:98` `--all` help row is now a regexp, because the longer `--currency code` column re-pads it.
- Mutations (all red as expected, restored): `Changed` guard removed -> `Test_currency_flag_is_checked_only_when_given` (all 5 subtests); validation disabled -> that test plus `..._refuses_a_value_it_cannot_read...` and `..._refused_before_the_command_checks_its_other_flags`; noArgs skipped -> `..._refused_after_the_check_for_a_positional_argument`; `doc.currency()` result dropped -> 7 config and cmd tests.
- `golangci-lint run ./...`: 0 issues. test-stats `--base b41dec1`: cmd/quarry 438 (+3), internal/cli 359 (+6), internal/config 58 (+9), internal/platform/money 11 (+3).
