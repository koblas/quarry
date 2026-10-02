# Review round 2: phase2f-fx (re-gate after fix pass 1)

- **Range:** 0e955e0..8bc1f7e.
- **Coverage gate:** 0 uncovered (1 declared unreachable, `cmd/quarry/main.go:11`, which the test reviewer accepted).
- **Mutation sample:** 8/8 killed.
- **Re-run:** arch, correctness-A, test-A (the reviewers that blocked). correctness-B, test-B and refactor were not re-run: they passed in round 1 and the fix touched nothing they own beyond the mechanical `testEnv` rename.

## Closed from round 1
- BLOCKER: tail interior gap.
- MAJOR: floor comes back after a rates fault.
- MAJOR: `newRatesSource` global.
- MAJOR: Valet decode reasons.
- MAJOR: inverted Need.
- MINORs: maxRate, seen-map, `history_test` name, `rates_test` `if`, refactor helpers.

## MAJOR
- **correctness-A, `internal/fx/plan.go:35`.** The head span is clipped to `earlier(need.Last, dayBefore(have.First))`. When Need lies wholly before Have, which needs the clock moved back past the first stored rate, `need.Last+1 .. have.First-1` is never asked. The floor then claims that range forever, and the ASOF join converts it at a stale rate with no warning. `fx_test.go:88` pins this. The STATE.md "unreachable" debt is wrong.
  - *Design smell:* this is the second pass on the same surface.
  - *Invariant to build to:* the asked spans and Have together form exactly one contiguous interval covering hull(Need ∪ Have), whenever Need is non-empty.
  - *Fix:* set the head span to `{need.First, dayBefore(have.First)}` and drop `earlier`.
  - *Test:* an exhaustive grid test (from test-A, specified below) plus a duckstore two-sync property.
- **test-A, `cmd/quarry/run.go:66`.** The shipped `duckstore.WithRates(fx.NewServer())` wiring is not pinned. Deleting the line survives (`go test ./cmd/quarry` passes). Add a `runProcess sync` test that runs with `HOME` set to a temp dir and `http.DefaultTransport` swapped for a recording RoundTripper answering an empty Valet body, with no `t.Parallel`. Assert that the FXUSDCAD request was seen.

## Invariant test (from test-A)
- **Grid:** day indices 0..6. Every Need (zero, inverted, and every First≤Last pair) × every Have (zero, and every First≤Last pair).
- **Driver:** `Refresh` through a `fakeSource` that records the spans asked on both series and answers one observation per asked day.
- **Assertions, derived from days rather than copied from `planSpans`:**
  - an empty or inverted Need asks nothing;
  - the asked spans are disjoint from each other and from Have, and come oldest first;
  - nothing is asked outside hull(Need ∪ Have);
  - when Need is non-empty, asked ∪ Have is exactly one contiguous interval covering Need ∪ Have;
  - if Have already covers Need, nothing is asked;
  - Rates dates plus Have form the same set, and `Added == len(Rates)`.
- **Also:** a duckstore two-sync property — after any second sync, the stored rate dates form one interval.

## MINOR / NIT: folded in
- `cmd/quarry/main_test.go:1`: the header is stale (`run` is now test wiring of `runProcess`).
- `cmd/quarry/main.go:9-11`: two near-duplicate unreachable comments; keep one.
- `internal/fx/fx.go:109`: key the seen-map by `Date.Format(time.DateOnly)`.
- `syncWithCoveringRate` seeds about 30k rows. No change required.

## Verdict: BLOCKED (0 BLOCKER, 2 MAJOR)
