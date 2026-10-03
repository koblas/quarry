---
id: SCENARIO-04
status: open
---

# SCENARIO-04: spending-trend.sql agrees with quarry spend (absorbs SCENARIO-05)

Cadence: code-first — no production Go; nothing on the mandatory set (only test code and two static `.sql` files)
Acceptance test: `cmd/quarry/run_skill_recipes_test.go` `Test_spending_trend_recipe_agrees_with_quarry_spend`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_skill_recipes_test.go` `Test_income_by_category_recipe_agrees_with_quarry_cashflow`
Narrow loop: `go test ./cmd/quarry/ -run 'spending_trend|income_by_category|recipe'` (matches every test this plan names)
Mutation checks: `starts_with(lower(category), lower(p.category) || ':')` → `lower(category) LIKE lower(p.category) || '%'` in `spending-trend.sql` → `Test_spending_trend_recipe_agrees_with_quarry_spend/category_food` (quote this behavioural red, not the static no-LIKE pin, which also reddens) · drop the native arm in `spending-trend.sql` (currency := param, amount := converted column only) → `Test_spending_trend_recipe_agrees_with_quarry_spend/pre_rate_split_on_native_row` · same in `income-by-category.sql` → `Test_income_by_category_recipe_agrees_with_quarry_cashflow` (USD 1000.00 row). Mutate the `.sql` files with the `proof.md` copy-aside protocol.
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`cmd/quarry` tests + 2 static `.sql` files)

## Implementation Plan

Reuse, do not copy: `repoFile` `run_plugin_manifest_test.go:71-77`; `chargeRows`/`chargeTxn` `run_helpers_test.go:169-216`; `spendRows` reference data `:124-170`; `spendEnv` `:83-85`; `day` `:219`; `replaceStoreWithRates` `run_sql_fx_test.go:32-36`; `usdRate`, `hardwareHistory`, `bigHardware` `run_anomalies_fx_edges_test.go:38-64`; `monthlySeries`, `inUSD` `run_recurring_json_test.go:83-100`; `salary` `run_analysis_documents_test.go:197-203`; `groceryCharge` `run_recurring_test.go:54-59`; `runSpendJSON`/`spendReport` `run_spend_fx_test.go:21-46`; Stdin pattern `run_sql_test.go:73-76`; decode into `document.SQL` `internal/report/document/sql.go:12-25`, `document.Spending` `spending.go:10-56`, `document.CashFlow` `cashflow.go:6-35`; `store.Transfer` `internal/store/store.go:122-130`; relations from `(*duckstore.Store).Schema` `schema_read.go:34`. Native arm mirrors `keepOwnCurrency` `internal/store/duckstore/convert_sql.go:24-27`.

### Acceptance (red)
- [ ] Step 1: `run_skill_recipes_test.go` (new) `Test_spending_trend_recipe_agrees_with_quarry_spend` — four subtests, one per outline row: `no_filter_year` (2022..2026, each year's currency-keyed totals = `spend --json --since Y --until Y` totals; require doc `currency == "CAD"` and a non-empty command side carrying the expected keys, 2025 incl. USD); `category_food` (2026, grain year, CAD; sum of spend `--by category` rows for literal set {Food, Food:Groceries, Food:Groceries:Organic}; Foodies spend row asserted present and outside the set); `shipped_values` (unmodified bytes; rows exactly (2022,CAD),(2023,CAD),(2024,CAD),(2025,CAD),(2025,USD),(2026,CAD),(2026,USD) in that order); `pre_rate_split_on_native_row` (payee 'Spotify', grain month, 2026-02-01..2026-03-31, CAD → exactly [{2026-02-01,USD,10.99},{2026-03-01,CAD,14.29}]). Plus `Test_income_by_category_recipe_agrees_with_quarry_cashflow` — 2026 CAD: currency-keyed sum = `cashflow --by year --since 2026 --until 2026` totals income; rows exactly [{(uncategorized),CAD,128.00},{Income:Salary,CAD,10130.00},{Income:Salary,USD,1000.00}]
- [ ] Step 2: `run_skill_eval_fixture_test.go` (new) `skillEvalStore(t, home)` stub builds a present, accounts-only store via `replaceStore` (commands exit 0); `run_skill_recipes_test.go` `recipeParams`, `runRecipe(t, file, recipeParams) document.SQL`, `runShippedRecipe(t, file)` stubs return a zero `document.SQL` without calling `repoFile` — red lands at the non-empty / equality assertions, not at a missing store or file

### Build
- [ ] Step 3: fixture + harness. `skillEvalStore` rows (below); runner: read the file via `repoFile` on every call, splice only the params line, feed `quarry sql --json -` through `Env.Stdin`, decode `document.SQL`; `Test_recipe_params_line_is_found_once_and_keeps_its_names` (controls: 0 lines, 2 lines, alias list in another order → helper error). Check, not a test: run `anomalies`, `recurring --since 2000`, `findings` once on `skillEvalStore`; only Hardware (anomaly), Netflix + Spotify (recurring), the Gas Bar pair + R12/R13 (duplicate, uncategorized) may appear — fix strays now (S06 is additive-only); record output in phase report
- [ ] Step 4: `plugin/skills/quarry/references/sql/spending-trend.sql` (new) — §S.7 contract; acceptance green
- [ ] Step 5: spending-trend arm pins in `run_skill_recipes_test.go`, one test per arm:
  - `Test_spending_trend_category_matches_the_subtree_ignoring_case` (2026, grain year, CAD) rows: 'FOOD' → Food set (lower(param)); 'food:groceries' → {Food:Groceries, …:Organic}, Food (ancestor) out; 'food:groceries:organic' → {…:Organic}, parent out — Foodies present-and-out in each
  - `Test_spending_trend_payee_matches_the_exact_name_ignoring_case` rows: 'Hardware', 'hardware', 'HARDWARE' same rows; 'Hard' → none; 'Hardware' + category 'Auto' → none
  - `Test_spending_trend_grain_month_matches_spend_by_month` (2026 vs `spend --by month`)
  - `Test_spending_trend_currency_usd_matches_spend_currency_usd` (2026 vs `--currency USD`, doc currency USD)
  - `Test_spending_trend_since_and_until_include_both_ends_only` (2024-06-10..06-20 → exactly [{2024-01-01,CAD,6.00}])
  - `Test_spending_trend_lists_cad_before_the_first_rate_natively_in_usd_mode` (payee Hardware, year, 2025..2026, USD → [{2025-01-01,CAD,200.00},{2026-01-01,USD,192.31}])
  - `Test_spending_trend_leaves_out_the_transfer` (payee 'Savings Sweep' → none; control: `search "Savings Sweep" --json` lists it flagged transfer)
  - `Test_spending_trend_columns_are_period_currency_spent` (`columns` = DATE, VARCHAR, DECIMAL(18,2))
- [ ] Step 6: `plugin/skills/quarry/references/sql/income-by-category.sql` (new) + S05 acceptance green + arms: `Test_income_by_category_currency_usd_matches_cashflow_currency_usd` (2026); `Test_income_by_category_since_and_until_include_both_ends_only` (2024-06-10..06-20 → [{Income:Interest,CAD,6.00}]); `Test_income_by_category_lists_cad_before_the_first_rate_natively_in_usd_mode` (2024, USD → [{Income:Interest,CAD,15.00}]); `Test_income_by_category_columns_are_category_currency_income` (VARCHAR, VARCHAR, DECIMAL(18,2)); `Test_income_by_category_runs_as_shipped` (unmodified bytes, exit 0, non-empty rows). Flow filter and `(uncategorized)` arms are pinned by the S05 acceptance's exact row list
- [ ] Step 7: Rule P4 static pins over both files: `Test_recipes_read_only_their_view` (every `Schema` relation except the own view absent at a word boundary; own view present as control); `Test_recipes_name_no_quicken_table_like_or_clock` (`\bZ[A-Z]`, `LIKE`/`ILIKE`, `current_date`/`now()`/`today()`); `Test_recipes_open_with_a_question_and_the_params_row` (line 1 `-- `, line 2 the params prefix); `Test_recipes_put_values_only_in_the_params_row` (quoted or `DATE '` literals off the params line ⊆ a named structural allowlist). Each scanner gets a crafted-text control (`FROM transactions`, `'Food'`) that it flags

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; 1-2 line doc comments on the new helpers

### Verify
- [ ] Step 9: full verification + `spec-check.py phase3d-skill` → tick SCENARIO-04 and SCENARIO-05 ("delivered by SCENARIO-04" before its test reference); STATE.md rewrite

**Fixture `skillEvalStore`** — accounts `chequingAccount("acct-cad",1)`, `usdChequingAccount("acct-usd",2)`, CAD `acct-savings`; add categories Food, Food:Groceries:Organic, Foodies (expense), Income:Salary, Income:Interest (income); every date ≤ 2026-09-29 (`spendEnv` clock). Row → what it feeds:
- R1 Netflix `monthlySeries` 2026-02..09 (999×4, 1199×4) → S06 new + price change; 2026 totals
- R2 `hardwareHistory()` + `bigHardware()` → S06 anomaly; USD-mode native CAD 200.00 / converted 192.31
- R3 `inUSD(monthlySeries("Spotify", 2025, Nov, 1099×7))` → native USD 10.99 (Feb) vs CAD 14.29 (Mar); 2025/2026 USD keys
- R4 `salary` 2026-03-01, 2026-04-01 (5000.00) → Income:Salary CAD; R5 `usdRate(2026-03-01, 1_300_000)`, the only rate
- R6 transfer "Savings Sweep" acct-cad −500.00 → acct-savings +500.00, 2026-04-15, uncategorized legs, `rows.Transfers` → transfer pin; (uncategorized) income stays 128.00
- R7 Food −11.00, Food:Groceries:Organic −22.00, Foodies −44.00 (2026-05-05/06/07, one-off payees) → category rows
- R8 Food:Groceries, no payee, 03-15 of 2022..2025 (distinct amounts), plus 2021-12-31 → shipped periods start 2022
- R9 Food:Groceries, no payee, 2024-06-09/10/20/21 at 1.00/2.00/4.00/8.00 → spending bounds (any arm flip changes 6.00)
- R10 Income:Interest on `acct-savings` (no duplicate pairing with R9) +1/+2/+4/+8 on the same four days → income bounds 6.00; USD-mode native 15.00
- R11 "Gas Bar" Auto:Fuel −37.00 on 2026-06-01 and 06-03 → S06 duplicate
- R12 uncategorized −64.00 2026-07-01 → in no-filter totals, out of every category row; S06 uncategorized
- R13 uncategorized +128.00 2026-07-02 → `(uncategorized)` income
- R14 "US Client" Income:Salary USD +1000.00 2026-02-15 (native) and +100.00 2026-04-15 (130.00 into CAD 10130.00)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Recipe line 1 is a `-- ` question comment; line 2 is the whole params CTE on one line, `WITH params AS (SELECT … AS category, … AS currency)` — the runner splices only that line and requires its ` AS <name>` list, in order, to match the shipped line; SKILL.md §5 "params row" means this line
- Recipe SQL is read from `plugin/` via `repoFile` on every run; no Go constant holds recipe SQL — the eval tests the shipped text
- Equality is currency-keyed maps on both sides, default spend/cashflow doc `currency == "CAD"` asserted, one USD cell; native rows compare on both sides
- Recipe output ordered by period, currency / category, currency; amounts cast to DECIMAL(18,2), period a DATE
- `skillEvalStore` (`run_skill_eval_fixture_test.go`) is S06's fixture: additive only; "Savings Sweep" never gets an ordinary split (S06 asserts it absent from `spend --by payee`)

**Unruled — orchestrator rules before dispatch:**
- Shipped `until` in `spending-trend.sql`: §S.7 names none. Plan uses `DATE '2026-12-31'`; as shipped it silently drops 2027+ data once that year arrives. Alternative: a far date (`9999-12-31`), which includes future-dated splits `spend` leaves out by default
- Shipped `since`/`until`/`currency` in `income-by-category.sql`: §S.7 names none. Plan uses `DATE '2026-01-01'`, `DATE '2026-12-31'`, `'CAD'` — same staleness as above

**Left unbuilt:**
- `references/*.md` prose and drift of recipe names via SKILL.md links — S03; use-case tests over `skillEvalStore` — S06

**Traps:**
- Empty command side equals empty recipe side: every equality first requires a non-empty command side with its currency keys
- Expected subtree sets are literal category lists; re-deriving them with prefix logic in Go copies the code under test
- `sum` widens DECIMAL and `date_trunc` returns TIMESTAMP — cast, or the `columns` pins and string compares fail
- `rows.ReferencedCategoryIDs` must stay sorted and unique when categories are added
- Step 7's word-boundary relation scan reads comments too: strip `--` lines before scanning, or line 1's question must avoid table names (`categories`, `payees`, `transactions`)
- New one-off payees or Food vs Foodies may raise anomaly, payee-variant or similar-category findings — Step 3's check exists for this
