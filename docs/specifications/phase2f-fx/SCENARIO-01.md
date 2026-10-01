---
id: SCENARIO-01
status: open
---

# SCENARIO-01: First sync back-fills exchange rates from the earliest transaction

Cadence: test-first — the import_runs rates UPDATE lands inside Replace's temp-then-rename (`rates.go:17-31`, called from `duckstore.go:319`)
Acceptance test: `cmd/quarry/run_sync_rates_test.go` `Test_run_sync_back_fills_rates_from_the_earliest_transaction`
Narrow loop: `go test ./internal/fx/ ./internal/store/duckstore/ ./internal/importer/ && go test ./internal/cli/ ./cmd/quarry/ -run 'Sync|Store|JSON|Usage|Rates'`
Mutation checks: import_runs rates UPDATE dropped from `finishBuild` → `Test_replace_records_the_asked_floor_and_the_last_rate_on_the_new_run`; UPDATE fault swallowed → `Test_replace_keeps_the_previous_store_when_the_run_cannot_record_its_rates`; parent-ctx check in `(*Server).Refresh` replaced by FetchError → `Test_refresh_returns_an_error_when_the_context_ends_during_a_fetch`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 new feature package (`internal/fx`) + duckstore Replace, importer pass-through, cli, cmd wiring; orchestrator ruled no SPLIT

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_rates_test.go` (new) `Test_run_sync_back_fills_rates_from_the_earliest_transaction` — v9fixture bundle with transactions back to 2005; env `NewServer = newServerFactory(duckstore.WithRates(fx.NewServer(fx.WithHTTPClient(<fake Valet RoundTripper>))))`; asserts fx_rates rows+series via `quarry sql` and the `Rates     USD/CAD <first> to <last> (<n> new)` line, exit 0
- [x] Step 2: stubs — `internal/fx/doc.go`, `fx.go` (`Server`, `Option`, `NewServer`, `WithHTTPClient`, `WithSource`, `Refresh` returning empty); `store.go:224-252` `RatesSummary{First, Last time.Time; Added int; FetchError string}` as `Result.Rates` and `Replaced.Rates`. Red at the fx_rates/Rates-line assertion

### Build
- [ ] Step 3: `internal/fx/source.go` `Source` port + `valet.go` adapter + `testdata/valet_fxusdcad.json`, `testdata/valet_iexe0101.json` (documented shape, see Traps) + `valet_test.go` over a fake RoundTripper. R7 pin: GET, path carries the series, query is exactly `start_date`+`end_date`, no body. Parse arms: observation without the series key skipped; rate `1.3456` → 1345600. Bounds: 6 decimals in / 7 out; 9999.999999 in / 10000 out (DECIMAL(10,6)); `0`, negative, non-numeric refused — a refusal fails the whole answer (FetchError via step 5), never drops the observation. Faults: `Do` error, non-200 status, body read error, JSON decode error, bad `d` date, bad rate
- [ ] Step 4: `internal/fx/plan.go` span planner + series choice, pinned through `Refresh` with a fake `Source` recording calls. Planner arms (spans, never date sets): Have zero → all Need; Have inside Need → head + tail; Have covers head → tail only; Have covers tail → head only; Have covers Need → no call; Need empty → no call; Need.First == Have.First and Need.Last == Have.Last+1 (one-day edges). Series arms — legacy is asked only for a span ending before Have.First, or for all of Need when Have is empty; a span after Have.Last asks FXUSDCAD only: FXUSDCAD covers the span → no legacy call; head/first-sync span with FXUSDCAD empty → IEXE0101 over the whole span; tail span with FXUSDCAD empty (weekend, unpublished) → no legacy call; IEXE0101 asked only for [span.First, first FXUSDCAD date − 1]; legacy date ≥ that cutover dropped; date in both → FXUSDCAD; head/first-sync span starting on a weekend still asks legacy for the weekend days; observation outside the asked span dropped; duplicate date within one series → first kept
- [ ] Step 5: `fx.go` `(*Server).Refresh` contract — `Test_refresh_returns_an_error_when_the_context_ends_during_a_fetch` (RoundTripper blocks until ctx done; classified by the PARENT `ctx.Err()`, never `errors.Is(err, DeadlineExceeded)`); every non-ctx Source failure → `FetchError` (placeholder reason, see Left unbuilt), nil error, no rates; `Added == len(Rates)`; Rates sorted by date, none inside Have, none ≤ 0, series set to the one it came from
- [ ] Step 6: `rates.go:17-31` `finishBuild` — after the fx_rates append, UPDATE the new (max id) import_runs row: `rates_first` = Need.First when the refresh has no FetchError (else NULL), `rates_last` = max(fx_rates.date); read min/max(fx_rates.date) into `Replaced.Rates` with Added/FetchError (`duckstore.go:348-351`); `faultDB` (`duckstore_test.go:479-493`) gains an Exec fault. Tests: `Test_replace_records_the_asked_floor_and_the_last_rate_on_the_new_run` (+ FetchError arm → NULL floor; no source → both NULL), `Test_replace_keeps_the_previous_store_when_the_run_cannot_record_its_rates` (Exec fault, real duckdb error shape), min/max query and scan faults. `importer.go:139-143` copies `replaced.Rates` into `Result.Rates` + importer test
- [ ] Step 7: cli + wiring. `render.go:89-99` `renderStore` Rates line after Findings: `(N new)` (humanize.Thousands) and `(up to date)` (Added 0, rates stored, no FetchError) arms. `json.go:35-44,172-183` `storeDocument.Rates` `{first,last,added,fetch_error}` after `findings`, dates `jsonDateLayout`, nulls when none. `sync.go:56` sync Long paragraph verbatim (spec *Changes to existing surfaces*). `run.go:24-30` guard `_ duckstore.RatesSource = (*fx.Server)(nil)`; `run.go:48-72` prepend `duckstore.WithRates(newRatesSource())` before storeOpts; package var `newRatesSource` → `fx.NewServer()`. `cmd/quarry/main_test.go` (new) `TestMain` sets it to a fixed-date fake (reuse `fakeRates`, `run_sql_fx_test.go:18-23`). Pins: renderStore arms (`render_internal_test.go:241-300`), Long verbatim (`run_usage_test.go:55-60`), `cmd/quarry/run_sync_rates_test.go` `Test_run_sync_json_reports_the_rates_it_fetched` — `--json` read back with `encoding/json` (first/last/added values, fetch_error null) + the key-set pin (`json_internal_test.go:40-130`); repoint every "Findings is the last line" pin to the Findings line (`run_sync_findings_test.go:41,81,297,320,351,368`, `run_findings_carry_test.go:77`)

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on fx exports, `Source`, `RatesSummary`, `newRatesSource`; fx doc.go names the Valet endpoint and the series rule

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2f-fx` → tick SCENARIO-01 with its acceptance test

## Handoff

**Binding decisions:**
- `import_runs.rates_first` = the earliest date a fetch with no FetchError asked for (Need.First). `rates_last` = the latest stored rate. Under this rule the permanent low gap (before IEXE0101's start, or a weekend before the first rate) is remembered and never refetched. 04 derives Have from the previous run's two columns, not from min/max(fx_rates). The head and tail differ on purpose: history never gains older days, but new days get published. **PROVISIONAL**, pending an orchestrator ruling. The spec's status "Store:" line reads as the plain rate span.
- The Rates line, `Replaced.Rates`, and status (06) take first/last from min/max(fx_rates.date), never from import_runs.
- The legacy cutover is data, never a constant. It is the first FXUSDCAD observation in that span's answer.
- `*fx.Server` is the adapter of `duckstore.RatesSource`. `fx.Source` is fx's own port to Valet, and it returns Go errors that Refresh turns into FetchError. fx never imports duckstore.
- Sync `--json` `rates` sits at `store.rates`, after `findings`, and is null when no store was built. **PROVISIONAL**: the name is ruled, the placement is not.
- `newRatesSource` is the one network seam in cmd/quarry. `TestMain` swaps it, and no cmd test reaches the network.

**Left unbuilt:**
- The 30 s per-request child timeout, the ruled `<reason>` copy (01's placeholder is `err.Error()`), the partial-range result, the `rates_fetch_error` write, warnings, and the `(not refreshed…)` / `none (not fetched…)` arms. All of these belong to 03.
- A real `Have` and fx_rates carry belong to 04. The status Rates line belongs to 06.
- The Rates line and JSON when nothing was fetched and nothing failed (Need empty, or an empty answer) have **no ruled copy**. The plan omits the text line and emits `rates` with nulls and added 0. This needs a product-vision ruling before B2.
- STATE open debt (the DuckDB `current_date` zone) is not closed here and is left to 12.

**Traps:**
- Classify by the parent `ctx.Err()`. Once 03 adds the child timeout, a `DeadlineExceeded` check would turn a timeout into "interrupted".
- 04: a FetchError run writes a NULL floor, so Have.First comes from the latest run whose `rates_first` is non-NULL, not simply the previous run. A partial result leaving Have.Last in the legacy era is the one case where a tail would need legacy — 03's.
- The TestMain fake now gives every cmd sync rates: existing import_runs pins that assumed NULL rates columns move (Sweep).
- Network data must never fail the build. Every PK or DECIMAL violation is filtered in fx (step 3/4 arms), because `rateRows` and fx_rates' PK+CHECK fail the whole sync.
- Unverified (the probe fetch was declined), so SCENARIO-20 confirms these:
  - the Valet URL `https://www.bankofcanada.ca/valet/observations/<series>/json?start_date=&end_date=`;
  - the JSON shape `observations[{d, <SERIES>:{v:"1.3456"}}]`, plus how a missing value is encoded;
  - that IEXE0101 exists, its start date, and its unit (CAD per USD, not the inverse);
  - that FXUSDCAD starts around 2017-01-03 and the series overlap;
  - whether an empty range is a 200 with no observations, or a 404.

## Phase report

Run A (steps 1-2) done. Acceptance is RED at its assertions, for the expected reason (stub Refresh returns nothing).

Files:
- `cmd/quarry/run_sync_rates_test.go` (new): `Test_run_sync_back_fills_rates_from_the_earliest_transaction`, plus helpers `fakeValet` (a `http.RoundTripper` that answers `/valet/observations/<series>/json?start_date=&end_date=` filtered to the range; `{"observations":[{"d":..,"<SERIES>":{"v":".."}}]}`; unknown series 404) and `valetResponse`. Reused by step 7's `--json` test.
- `internal/fx/{doc,source,fx}.go` (new, stubs): `Server`, `Option`, `NewServer`, `WithHTTPClient(*http.Client)`, `WithSource(Source)`, `Refresh` returns `store.RatesRefresh{}`. `source.go` already declares `Source{Observations(ctx, series string, span store.DateSpan) ([]Observation, error)}` and `Observation{Date, Rate}` because `WithSource` needs the type. That shape is B1's to refine.
- `internal/store/store.go`: `RatesSummary{First, Last time.Time; Added int; FetchError string}` added as `Result.Rates` and `Replaced.Rates` (not yet filled or rendered; the `Result`/`Replaced` doc comments do not mention it yet).

Red output (`go test ./cmd/quarry/ -run Test_run_sync_back_fills`):
- line 78: stdout lines (ending `Findings  1 open; ...`) do not contain `Rates     USD/CAD 2005-03-01 to 2017-01-04 (5 new)`.
- line 82: `quarry sql --csv` of fx_rates is the header only, expected 5 rows.

Test shape: bundle with transactions 2005-03-01 and 2026-03-01; fake Valet serves IEXE0101 2005-03-01, 2005-03-02, 2017-01-02 and FXUSDCAD 2017-01-03, 2017-01-04. Expected fx_rates is those 5 rows (usd_cad printed `1.234500`, DECIMAL(10,6) format guessed from the 2-decimal views, so confirm it at green). Last = 2017-01-04 is deterministic whatever today is.

Asserts only the ruled `(N new)` text line and fx_rates contents. It does not assert `rates_first`, the JSON `rates` placement, or the nothing-fetched copy (pending ruling).

Lint on touched packages: 0 issues. Narrow loop for B1: `go test ./internal/fx/`. B2 and the acceptance test stay red until step 6/7 render the Rates line and step 5 fills the rates; the fx_rates rows go green after steps 3-5.
