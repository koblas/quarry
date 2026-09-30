---
id: SCENARIO-18
status: done
---

# SCENARIO-18: findings filters by status and type

Cadence: code-first (no write-safety, atomicity or bug-fix item touched)
Acceptance test: `cmd/quarry/run_findings_filters_test.go` `Test_run_findings_status_all_type_duplicate_marks_ignored_shows_fixed_as_a_date_line_and_counts_duplicates_only`
Narrow loop: `go test ./internal/report/ ./internal/cli/ -run 'findings|Findings'` then `go test ./cmd/quarry/ -run 'run_findings'`
Mutation checks: W1 from the unfiltered rows → `Test_findings_reports_unmatched_ignore_ids_even_when_type_filters_their_finding_out`; counts follow `--type` → `Test_findings_counts_follow_the_type_filter_not_the_status_filter`; ignored-and-fixed is fixed → `Test_findings_status_ignored_omits_an_ignored_finding_that_is_also_fixed`
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (`report`) + cli

Surveyed, not re-planned: `--status`/`--type` flags, values validation, `Classify`, W1 wiring, config load, default-view footer and hint all exist (STATE.md). Callers of `renderFindings`/`findingsFooter`/`renderFindingsJSON`/`FindingsRequest`: only `internal/cli/findings.go:87-98` plus their own tests (grep, production).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_findings_filters_test.go` (new) acceptance test — fixture: sync three duplicate pairs (A open, B ignored via `writeConfig`, C later fixed by `syncNewBundle` of a bundle without C) plus the Amazon uncategorized payee (`run_findings_ignore_test.go:26-47`, `:150-176` show both moves); pin C's `fixed_at` with `editStore` (`run_read_refusals_test.go:192`) to noon UTC on a fixed day; run `--status all --type duplicate`. Asserts stdout in full (header without fix clause only if no open finding; here A is open so the fix clause shows), `  <B>  ignored`, `  <C>  fixed <local date>`, footer `3 findings: 1 open, 1 ignored, 1 fixed`, hint absent (ignore set), no uncategorized group. Compiles today; must fail at its assertion (filters not passed on).

### Build
- [x] Step 2 (B1, batch 1): `internal/report/findings.go:19-77` `FindingsRequest`, `FindingsGroup`, `(*Server).Findings`, `findingOrder` — request carries the parsed status (open|ignored|fixed|all) and type ("" = all); each listed finding carries its `finding.Status` (developer picks the least-churn shape; `idsOf`/`Findings []store.Finding` users in `report/findings_test.go` and `cli` render funcs move with it); groups hold every finding whose status is selected, in `finding.Types()` order, empty groups omitted; within a group non-fixed findings in the existing type order, then fixed by `FixedAt` desc then id (`findingOrder` stays the open/ignored order). `Classify` called twice: over ALL known-type rows for `Statuses` + `Unmatched` (W1 never follows the filter), over the `--type` subset for `Counts` only (same ignore list; no second ignore map, P2d-3). Tests in `report/findings_test.go` (near `:111-156`, `:191-213`): each of the four statuses; type filter; counts follow type not status; unmatched unfiltered (mutation); ignored+fixed appears under fixed only and not under ignored (mutation); fixed sort incl. equal-`fixed_at` id tiebreak; fixed after open/ignored inside an `all` group; unknown-type rows still neither listed nor counted under a filter. No new fallible call: the existing `Test_findings_refuses_with_the_store_refusal_copy` covers `store.Findings`.
- [x] Step 3 (B1, batch 2): `internal/cli/render_findings.go:26-46`, `:62-83`, `:107-159`, `:179-195` `renderFindings`, `findingsHeader`, `findingLines`, `oneSidedFindingRows`, `uncategorizedRows`, `findingsFooter` — `renderFindings(listing, view, showHint)` where view = status + type; markers and lines below; one footer func per view; padding widths computed over non-fixed rows only; fixed line never padded. Tests in `render_findings_internal_test.go` (`:169-228`): one table case per ruled string below, each edge row its own case (singular `1 ignored finding`/`1 fixed finding`/`1 finding: 1 fixed`, zero clauses omitted, `--type` empty lines, ignored marker on duplicate / one-sided / uncategorized / no-layout rows, fixed line under a padded group, header with no open finding loses its fix clause but keeps `(N)`, hint not printed when no open finding is listed); `time.Local` swapped for a fixed zone (restored by `t.Cleanup`, no `t.Parallel`) so the local date of `fixed_at` is pinned, with a timestamp whose UTC and local dates differ.
- [x] Step 4 (B2): `internal/cli/findings.go:87-98`, `internal/cli/json_findings.go:48-76` `renderFindingsJSON`, `newFindingEntryDocument` — pass `status`/`type` into `FindingsRequest`; hint iff `len(req.Ignore)==0` AND an open finding is listed (status open|all, `Counts.Open > 0`); JSON document `status` = flag value (incl `all`), `type` = flag value or null, entry `status` from the finding, `fixed_at` = `jsonTimestamp` (UTC RFC3339) for fixed else null, fixed `items` `[]`, `counts` from the type-filtered tally. Tests: `cli/findings_test.go` flag-to-request pass-through against `fakeReportStore` (`:156-183`); `cmd/quarry/run_findings_json_test.go` (`:26-148` fixtures) `--status all --type duplicate` document; run-level `--status fixed` and `--status ignored` text (footer `1 fixed finding` / `1 ignored finding`); W1 still printed for an unmatched id under `--type duplicate`.

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; update doc comments that still say "open findings" (`report/findings.go:23-41`, `cli/findings.go:20`, `render_findings.go:24,177`).

### Verify
- [x] Step 6: full verification + `.claude/scripts/spec-check.py phase2d-findings` → tick SCENARIO-18 with its acceptance test; rewrite STATE.md (drop the 18 items from Left unbuilt and the hardcode note on `--json`)

## Handoff

**Ruled copy pinned** (Surface & Copy): `  <id>  ignored` (under `--status all`), `  <id>  fixed 2026-10-01` (local date of `fixed_at`, no items), footers `21 findings: 5 open, 4 ignored, 12 fixed`, `N ignored findings`, `N fixed findings`, `No open findings of type duplicate`, `No findings of type duplicate`, `No fixed findings`, hint text unchanged.

**Derived, NOT ruled — copy ruling requested before dispatch** (plan implements these strings; change is one constant each):
- `--status all`, nothing at all: `No findings`; `--status ignored`/`fixed` with or without `--type`: `No ignored findings`, `No ignored findings of type duplicate`, `No fixed findings of type duplicate`.
- Default view with `--type X`, none open but ignored/fixed of X exist: `No open findings of type duplicate; 1 ignored not shown (--status all)` (footer rule "counts follow --type" applied to the empty line).
- Group header with no open finding listed: heading and `(N)` only, no `: fix` clause (spec: clause "only when the group lists >= 1 open finding"); uncategorized `(N payees, M splits)` counts items only, so fixed ones add 0 splits.
- Marker on a row-style id line (one-sided, uncategorized): appended after the last column as `  ignored`; fixed findings follow open/ignored ones in their group.
- Hint only when an open finding is listed (so never under `--status ignored|fixed`).

**Binding decisions**: W1 `Unmatched` is always computed over every known-type row; only `Counts` and listing follow `--type`. `Classify` stays the sole matcher (called twice, never a second map). 19's `status` counts and 22's CSV (same filters, fixed = one row, item fields empty) reuse this request/listing shape.

**Left unbuilt**: `--csv` (22); item `category`/`transactions`/`splits` (24-28); `status` Findings line (19).

**Traps**: a fixed finding has no items, so `latestDate`/`payeeOf`-based sorts must not run on it; `fixed_at` is UTC in the store and `--json`, local only in text.

## Phase report

Run V done. Sweep: only `report/findings.go` `findingOrder` comment still said "open findings" (now open and ignored; fixed sorts apart); the other named comments were already updated by B1/B2. `go build ./...` ok, `golangci-lint run ./...` `0 issues`. Covered full suite rc=0, `uncovered-diff` 0 uncovered since 1e8c079, `-race` ok on `internal/report`, `internal/cli`, `cmd/quarry`. `test-stats --base 1e8c079`: cmd/quarry 324 (+5), internal/cli 196 (+13), internal/report 98 (+11), TOTAL 618 (+29). SCENARIO-18 ticked in `specification.md` with its acceptance test; `spec-check.py phase2d-findings` OK; STATE.md rewritten (18 items out of Left unbuilt, Classify-twice, markers and fixed-line traps in).
