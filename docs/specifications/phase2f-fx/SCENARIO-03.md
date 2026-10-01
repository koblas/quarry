---
id: SCENARIO-03
status: open
---

# SCENARIO-03: Failed rate fetch warns and the sync still succeeds

Cadence: test-first — write-safety: a failed fetch still swaps the new store in, a parent cancel keeps the previous one
Acceptance test: `cmd/quarry/run_sync_rates_fetch_test.go` `Test_run_sync_warns_and_swaps_the_store_in_when_the_rate_fetch_fails`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_sync_rates_prefetch_test.go` `Test_run_sync_makes_no_rate_request_when_it_fails_before_the_swap`
Narrow loop: `go test ./internal/fx/ ./internal/store/duckstore/ ./internal/snapshot/ ./internal/cli/ && go test ./cmd/quarry/ -run 'Rates|Prefetch|Fetch'`
Mutation checks: parent-`ctx.Err()` check `(*Server).Refresh` fx.go:52 → `Test_refresh_returns_an_error_when_the_parent_ends_during_a_timed_request`; timeout arm of `fetchReason` → `Test_refresh_gives_the_timeout_reason_only_after_30_seconds`; keep-on-failure in `Refresh` → `Test_refresh_keeps_the_rates_of_the_spans_that_answered`; body cap in `(*Valet).get` → `Test_valet_refuses_an_answer_one_byte_over_the_cap`; `rates_fetch_error` arg in `recordRates` → `Test_replace_records_the_fetch_reason_on_the_new_run`; FetchError-is-not-an-error in `finishBuild` → `Test_replace_swaps_in_the_store_when_the_rate_fetch_fails` (rates_test.go:125)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches; fx + store/duckstore + snapshot + cli (+ cmd tests); orchestrator's sizing verdict stands, folds 05

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_rates_fetch_test.go` (new) `Test_run_sync_warns_and_swaps_the_store_in_when_the_rate_fetch_fails` — table, one row per Examples row; harness `syncThrough`-style with an `http.RoundTripper` per row (run_sync_rates_carry_test.go:61). Rows: unreachable (RoundTrip returns `&net.OpError{Op:"dial"}`, which `client.Do` wraps in `*url.Error`; no prior store → `Rates     none (not fetched; see warning)` + nothing-stored warning); timeout (RoundTrip returns `context.DeadlineExceeded` at once, prior store with rates → `(not refreshed; see warning)` + nothing-new warning); 503 and unparseable (one per arm); partial (first sync with a transaction dated before the first FXUSDCAD observation, else IEXE0101 is never asked; FXUSDCAD answers, IEXE0101 503 → current rates kept, partial warning, `(not refreshed; see warning)`). Each row: exit 0, stderr line verbatim, Rates line verbatim, `import_runs` gained a row whose `rates_fetch_error` = reason (via `quarry sql`)
- [x] Step 2: `internal/fx/reason.go` (new) unexported reason constants + `fetchReason` signature-only; `internal/store/store.go:262-266` `RatesSummary.Partial`; `internal/snapshot/import.go:24-45` `Outcome.fetchWarning` field — stubs only; red at its stderr assertion

### Build
- [ ] Step 3: fx reasons, timeout, body cap. `valet.go:29-32` replace `errStatus` with `statusError{Code}` and add `errUnreachable`, `errNotRates`; `valet.go:58-81` `get` wraps `client.Do` fault → errUnreachable, non-200 → statusError, body via `io.LimitReader` capped at `maxAnswerBytes` (over cap → errNotRates), body read fault → errUnreachable; `valet.go:84-102` parse faults → errNotRates; `fx.go:86-98` `observe` runs each Source call under a child `context.WithTimeout(ctx, requestTimeout)` (30 s) and wraps `checkRate` fault (fx.go:97) in errNotRates; `fx.go:52-57` classify: parent `ctx.Err()` → error (interrupted), then child ctx `Err()` is DeadlineExceeded OR `errors.Is(err, DeadlineExceeded)` → timeout copy (wrap sites keep the cause), then `fetchReason(err)`: statusError → `www.bankofcanada.ca answered <code> <http.StatusText>` (built from StatusCode, never `resp.Status`; empty text → code alone), errNotRates → not-a-list, errUnreachable and `default:` → `cannot reach www.bankofcanada.ca`. Tests (fx_test / valet_test): `Test_refresh_gives_each_failure_its_ruled_reason` (rows: dial `*url.Error{Err:*net.OpError}` via fake RoundTripper, 503, 599-no-text, bad JSON, impossible rate fx.go:97, body read fault, plain error from fake Source = default arm); `Test_refresh_gives_the_timeout_reason_only_after_30_seconds` (synctest + RoundTripper selecting on `req.Context().Done()` vs a timer: answers just before 30 s → rates, just after → timeout reason; row: headers at once, body blocks until `req.Context().Done()` then returns `io.ErrUnexpectedEOF` → timeout reason); `Test_refresh_returns_an_error_when_the_parent_ends_during_a_timed_request` (synctest, parent cancelled at 10 s → error wraps Canceled, no FetchError) — its other half, previous store byte-identical on a ctx error, is already pinned by `rates_test.go:153` `Test_replace_does_no_build_work_after_a_rate_fetch_the_context_interrupted`; `Test_valet_accepts_an_answer_exactly_at_the_cap` (valid answer whitespace-padded to `maxAnswerBytes`) / `Test_valet_refuses_an_answer_one_byte_over_the_cap`. Re-point `fx_test.go:224`, `:260`, `:292` (DeadlineExceeded with live parent = timeout reason) to ruled reasons; `valet_test.go:148,169,177,195` to the sentinels
- [ ] Step 4: partial range + record. `fx.go:48-61` a failed span no longer drops kept rates: later spans are still asked, FetchError = first failure's reason, `Partial` = FetchError and ≥1 rate kept, `Added == len(Rates)`; `fx.go:65-82` `fetch`: FXUSDCAD answered + IEXE0101 failed → return current's rates with the error. `duckstore/rates.go:25-26,46-47,106-112` UPDATE also sets `rates_fetch_error` (NULL when empty), summary carries Partial. Tests: flip `fx_test.go:245` to `Test_refresh_keeps_the_rates_of_the_spans_that_answered` (head ok/tail fails and head fails/tail ok, `spansAsked` pins tail still asked); `Test_refresh_keeps_the_current_rates_when_the_legacy_series_fails`; `Test_refresh_reports_the_first_reason_when_two_spans_fail_differently`; `Test_refresh_returns_an_error_when_the_parent_ends_after_a_failed_span`; `Test_refresh_is_not_partial_when_the_answered_span_was_empty`. duckstore: `Test_replace_records_the_fetch_reason_on_the_new_run` (control: success → NULL), extend `rates_test.go:289` table with the column; `rates_floor_test.go` `Test_replace_keeps_the_checked_floor_on_a_partial_fetch` (head ok, tail fails → floor = previous, pins rates.go:87-92 as built); `Test_replace_reports_a_partial_fetch_in_its_summary`
- [ ] Step 5: warnings + Rates arms. `snapshot/import.go` new `fetchWarning(store.RatesSummary) string` (3 ruled lines, dates `time.DateOnly`), set at `import.go:166` beside `carryWarnings`, appended in `warnings` `import.go:62-65` after ratesWarning, before prune; `Warnings` doc `import.go:47-49` lists it. `cli/render.go:102-127` / `cli/json.go:197-211` need no change (partial → `(not refreshed; see warning)`, `added: n`). Tests: `snapshot` `Test_fetchWarning` (nothing stored / nothing new / partial / no FetchError → empty); `auto_prune_faults_test.go:106` sibling `Test_outcome_lists_the_fetch_warning_after_the_rates_warning_and_before_prune` (both `Warnings` and the absolute variant); cmd `run_sync_rates_fetch_test.go` `Test_run_sync_json_carries_the_fetch_reason_in_warnings_and_rates` (rows: nothing stored, nothing new, partial with `added` = n; `warnings[]` element and `store.rates.fetch_error` verbatim); `Test_run_sync_prints_the_carry_warning_then_the_fetch_warning` (unreadable fx_rates + failing fetch, text and `--json`); `Test_run_sync_prints_the_combined_carry_line_then_the_fetch_warning`; `Test_run_sync_from_warns_when_the_rate_fetch_fails` (`--from` reaches importVerified via from.go:72). Grep `fakeValet{` users (run_sync_rates_test.go, run_sync_rates_floor_test.go, run_sync_rates_carry_test.go) for a missing series whose 404 used to drop everything; fix expectations those now keep
- [ ] Step 6: folded 05. `cmd/quarry/run_sync_rates_prefetch_test.go` (new) `Test_run_sync_makes_no_rate_request_when_it_fails_before_the_swap` — counting `duckstore.RatesSource` via `newServerFactory(duckstore.WithRates(counter))`; rows: not open (`v9fixture.ClosedWALBundle`), validation failed (`unreconciledBundle` run_sync_prune_test.go:49), schema changed (`v9fixture.MissingSchemaBundle`); control row: good bundle → exactly 1 call. Each row: existing exit code, 0 calls, no line starting `Rates ` on stdout. `Test_run_sync_json_makes_no_rate_request_when_it_fails_before_the_swap`: validation failed → `store.rates` null; not open / schema → no `store`. duckstore `Test_replace_asks_for_no_rates_when_the_build_fails` (append fault via `faultDB`, 0 Refresh calls)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (fx.go:56 `//nolint:nilerr` stays only if still needed); doc comments on `Refresh` (fx.go:44-47: partial keep, reasons), `Observations` (cap), `RatesSummary.Partial`, `fetchWarning`, `Outcome.Warnings` order; `go doc ./internal/fx`

### Verify
- [ ] Step 8: full verification + `.claude/scripts/spec-check.py phase2f-fx` → tick SCENARIO-03 with its acceptance test; tick SCENARIO-05 `— delivered by SCENARIO-03 —` with its folded acceptance test; rewrite STATE.md (drop the Valet body-cap debt and the legacy-tail trap)

## Handoff

**Binding decisions:**
- Body cap `maxAnswerBytes` = 16 MiB (`16 << 20`): the largest real answer is one series over ~60 years ≈ 16k observations × ~100 B ≈ 1.6 MB; 10× headroom, still bounds memory. Over cap = `… not a list of exchange rates` — proposed number, orchestrator may overrule
- The 30 s timeout is a child ctx per Source call in `(*Server).observe`, so it covers every Source; Valet has none of its own. Classification order: parent `ctx.Err()` (interrupted) first, then the child ctx expired OR `errors.Is(DeadlineExceeded)` = timeout (so a stalled body whose read error drops the cause is still a timeout) — a fake returning DeadlineExceeded with a live parent is a timeout
- Reason mapper `default:` (unclassified Source error, body read fault) = `cannot reach www.bankofcanada.ca`; status copy from `StatusCode` + `http.StatusText`, never `resp.Status`
- Partial keep is per Source answer: a failed FXUSDCAD answer keeps nothing of its span (legacy never asked); a failed IEXE0101 keeps the span's FXUSDCAD rates. Kept rates are therefore always adjacent to Have or start a fresh Have at the cutover, so Have stays one interval with no interior gap and Have.Last never lands in the legacy era (retires STATE's legacy-tail trap)
- Later spans are still asked after a failure (≤3 requests, ≤90 s worst case); FetchError = first failure's reason
- `rates_checked_from` never advances on any FetchError, partial included (`askedFrom` rates.go:87-92): a partial never claims the failed span; the answered head is re-asked once next sync
- `Partial` reaches the warning through `store.RatesSummary.Partial`; the Rates line ignores it

**Needs a copy ruling (unruled, flagged):**
- Fetch warning order: planned manifest, history, findings, rates-carry, **rates-fetch**, prune
- Partial-range Rates line: planned `(not refreshed; see warning)` per the Gherkin Then, while `--json` `added` = n — text and JSON disagree; a ruling may prefer `(<n> new)` there

**Left unbuilt:** status Rates line and its failure clause from `rates_fetch_error` — 06

**Traps:**
- cmd `valetResponse` (run_sync_rates_test.go:46) sets `Status` without the code; reading `resp.Status` passes in prod and fails in tests
- A synctest fake RoundTripper that ignores `req.Context()` never times out — the outside-bound case would pass as success

## Phase report

Run A done (steps 1-2).

Files: `cmd/quarry/run_sync_rates_fetch_test.go` (new: acceptance test, helpers `roundTripFunc`, `failing`, `answering`, `legacyDown`, `syncCapturing`, `lastFetchError`, reason/tail consts, `januaryBank`); `internal/store/store.go:262` `RatesSummary.Partial` (exported field, unused until B2).

Red, at assertions, all 5 rows: stderr `[]string{""} does not contain "quarry: warning: could not fetch exchange rates from the Bank of Canada: ..."`; `rates_fetch_error` expected `\ncannot reach www.bankofcanada.ca\n` got `\n""\n` (NULL, nothing written); partial row also `date\n2017-01-03\n2017-01-04\n` expected, got `date\n` (a failed span still drops kept rates).

Deviations:
- Step 2 `fx/reason.go` `fetchReason` and `Outcome.fetchWarning` stubs NOT added: nothing references them, and golangci-lint `unused` rejects both. B1 creates `fetchReason`, B2 the field.
- Rates-line asserts are green on arrival for the two ruled arms (`not refreshed` / `not fetched`; already rendered); they stay in the table. Partial row leaves `ratesLines` nil (copy ruling pending), and no assertion pins warning position (stderr is checked by line containment).
- The 503 and unparseable rows differ from the plan only in which have an earlier store (503 earlier, unparseable none).

Next runs must not redo: the test file's `roundTripFunc`/`answering`/`failing` live in package main (cmd); B2's `Test_run_sync_json_carries_...` can reuse `syncCapturing`, `januaryBank`, the consts. NULL in `quarry sql --csv` prints `""`.
