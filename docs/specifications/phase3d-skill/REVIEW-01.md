# Review 01 — phase3d-skill

## Target
Changed files, range `977519372acb..HEAD`. Gate inputs:
- Full suite: `go test rc=0`.
- `uncovered-diff`: no production Go lines added.
- `test-stats`: cmd/quarry 660 (+57).
- Mutation sample: skipped, because no production `.go` file changed.

## Triggered reviewers
arch-reviewer, correctness-reviewer and refactor-advisor (on `cmd/**/*.go`), and test-reviewer (on `**/*_test.go`). The correctness reviewer was also given the plugin's SQL and prose.

## Skipped reviewers
api-reviewer (no HTTP surface) and pipeline-reviewer (no `.claude/**` changes).

## BLOCKER
None.

## MAJOR
1. **Recipe currency fallback** (correctness-reviewer). Files: `plugin/skills/quarry/references/sql/spending-trend.sql:16-17`, `income-by-category.sql:5-6`.
   - The bug: `CASE p.currency WHEN 'CAD' THEN …_cad ELSE …_usd END` gives every value other than `'CAD'` USD-converted amounts, labelled with the param.
   - Probe: with `'native'`, CAD and USD amounts are converted, added together and labelled "native" (`Income:Salary native 8792.30`). That is a silently wrong total. `'cad'` gives the same result.
   - Fix:
     - In both recipes, change both the amount and the label expression to `WHEN 'CAD' … WHEN 'USD' … END`, with no ELSE. Any other value then falls through to per-split native rows.
     - Add `'USD'` to `recipeLiterals`.
     - Add a test row for an out-of-domain value (`'native'`) that asserts per-currency native rows.
2. **Trend partial periods** (correctness-reviewer). File: `plugin/skills/quarry/references/spending.md:48-50`.
   - The bug: the trend guidance never says that the first and last periods can be partial, yet it says "To compare two years, quote both rows and show the difference". The shipped grocery question compares 9 months of 2026 with full years.
   - Fix: add a bullet that mirrors `cash-flow.md:19`, telling Claude to say when the first or last period is partial and not to compare a partial period with a whole one. Pin it in `Test_reference_files_state_their_job`.
3. **P4 recipe registry not tied to disk** (test-reviewer). File: `cmd/quarry/run_skill_recipes_test.go:487` (`recipeFiles`, `recipeLiterals`, `shippedParamsLine`, `recipeParamNames`).
   - The bug: the Rule P4 static pins iterate hand-kept maps. A new `.sql` file under `references/sql/` with `FROM transactions`, `LIKE` or `now()` passes every test.
   - Fix: collapse the maps into one `[]recipeSpec` registry, and add a test asserting that the registry's file names equal the `.sql` files on disk.

## MINOR
- arch: `cmd/quarry/run_skill_schema_reference_test.go:24`. The package-global `-update` flag applies to the whole `cmd/quarry` test binary.
- correctness: `spending.md:330` and `cash-flow.md:168`. The recipe `since`/`until` params need `DATE 'YYYY-MM-DD'` literals; a quoted string fails with a Binder Error.
- correctness: `recurring-and-anomalies.md:24`. "`currency` is the reporting currency" is not always true: an unconverted series stays in its own currency.
- correctness: `recurring-and-anomalies.md:10`. "Changes most times" is wrong; the rule is a change of more than 5% on more than a quarter of steps (`pricechange.go:12,55`).
- test: `run_skill_references_test.go:145`. `Test_references_name_no_mcp_tool` has no non-empty guard on the tool list.
- test: `run_skill_json_fields_test.go`. The field checks are one-directional. The prose is not checked against the declared fields, keys can match at any depth, and mentions are counted file-wide.
- test: `run_skill_recipes_test.go:276`. The `if c.err` branch in the test body should be split into two tables.
- test: `run_status_json_test.go:166`. The `if !p.nullable` branch in the subtest body.
- test and refactor: duplicated helpers:
  - `collapseWhitespace` and `collapsed`;
  - `skillMDPath` and `skillPath`;
  - `recipeDir` and `referencesDir`;
  - two README section extractors with different end markers (`run_skill_drift_test.go:51` stops at `## Credits`, the README test at the next `## `);
  - `skillRunCell` re-cuts §4 itself;
  - two fence parsers (`codeUnits`, `sqlOf`);
  - two comment strippers (`linesWithout`, `withoutCommentLines`);
  - two link extractors.
- test and refactor: the `"../../"` prefix is hand-built outside `repoFile` (manifest_test:41,64; drift_test:64,69; drift_names_test:175).
- test: the run-and-decode-JSON boilerplate is repeated 5+ times, and the tests mix `context.Background()` with `t.Context()`.
- test: tests that add to the count without adding proof: `Test_income_by_category_runs_as_shipped`, the name/command asserts in `Test_plugin_manifests_agree…`, `Test_skill_schema_reference_does_not_name_a_view_the_store_lacks`, and the three `carries_*` tests (subsumed by the golden).
- test: inconsistent setup and pin forms. Two recipe tests hand-roll HOME. `storeRelations` builds a full fixture. Bare-word phrase pins are used. `helpTree` is rebuilt four times.
- test: `run_skill_recipes_test.go` is 652 lines. Split the static pins into their own file.
- test: the package-main justification comments give the wrong reason.
- refactor: `skillEvalRows` is 55 lines; split it by kind.
- refactor: `Test_drift_check_flags_crafted_name_text` is 87 lines, with closures inside its cases.
- refactor: extract a `statusJSON(t, home)` helper.

## NIT
- arch: the repeated `"../../"` literal (see above).
- correctness: `recurring-and-anomalies.md:21`. The ended thresholds are "more than N days".
- correctness: `recurring-and-anomalies.md:34`. Say "for example" before the `not_judged` example.
- correctness: SKILL.md §1 templates can render `null` for `dates.last`/`rates.last`. This is ruled copy and would need a product-vision ruling.
- correctness: `income-by-category.sql:4`. COALESCE merges a real category named "(uncategorized)" into the NULL row.
- correctness: `--since 2000` leaves out series that ended before 2000.
- refactor: `lastNonEmptyLine` has no doc comment. Rename `recipeScenario`. Doc comments use the jargon word "Ruled copy". The schema `-update` code is inline (an existing debt).
- test: one big byte-pin test. `Test_references_name_no_phase_4_view…` has a misleading name. The `quarry accounts` row has no eval.

## Strengths
- Every scanner has a crafted-text control table.
- Every `Empty(mismatches)` has a positive `resolved` assertion beside it.
- `spliceParams` refuses a missing params line, a doubled one, or changed aliases.
- The recipe-equals-command evals cannot pass vacuously.
- Finding types are read from the live `--help` output.
- The reference prose matches the code on thresholds, defaults and finding fixes (correctness-reviewer checked this).

## Verdict: BLOCKED
