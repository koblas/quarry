// The recipes are read by repo-relative path, so these tests live in package main
// beside the other cmd/quarry tests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recipeDir         = "plugin/skills/quarry/references/sql/"
	spendingTrendFile = "spending-trend.sql"
	incomeByCatFile   = "income-by-category.sql"
)

// recipeParams are the values spliced into a recipe's params row; "" category or payee is NULL.
type recipeParams struct {
	category, payee, grain, since, until, currency string
}

// recipeParamsPrefix opens the one line of a recipe that holds its values.
const recipeParamsPrefix = "WITH params AS (SELECT "

// recipeParamNames are the aliases of each recipe's params row, in order.
var recipeParamNames = map[string][]string{
	spendingTrendFile: {"category", "payee", "grain", "since", "until", "currency"},
	incomeByCatFile:   {"since", "until", "currency"},
}

var (
	errNoParamsLine   = errors.New("no params line")
	errTwoParamsLines = errors.New("two params lines")
	errParamAliases   = errors.New("params aliases differ")
)

// paramAlias matches the alias that closes one value of a params row: after it comes a comma or the line's end.
var paramAlias = regexp.MustCompile(` AS (\w+)(?:,|\)$)`)

// spliceParams is sql with its params line replaced by p, which must hold exactly one such line
// carrying the aliases in names, in that order.
func spliceParams(sql string, names []string, p recipeParams) (string, error) {
	lines := strings.Split(sql, "\n")
	at := -1
	for i, line := range lines {
		if !strings.HasPrefix(line, recipeParamsPrefix) {
			continue
		}
		if at >= 0 {
			return "", fmt.Errorf("%w, lines %d and %d", errTwoParamsLines, at+1, i+1)
		}
		at = i
	}
	if at < 0 {
		return "", errNoParamsLine
	}
	var aliases []string
	for _, m := range paramAlias.FindAllStringSubmatch(strings.TrimRight(lines[at], " "), -1) {
		aliases = append(aliases, m[1])
	}
	if !slices.Equal(aliases, names) {
		return "", fmt.Errorf("%w: %v, want %v", errParamAliases, aliases, names)
	}
	values := make([]string, len(names))
	for i, name := range names {
		values[i] = p.sql(name) + " AS " + name
	}
	lines[at] = recipeParamsPrefix + strings.Join(values, ", ") + ")"
	return strings.Join(lines, "\n"), nil
}

// sql is the SQL literal for the value of the param called name.
func (p recipeParams) sql(name string) string {
	text := map[string]string{
		"category": p.category, "payee": p.payee, "grain": p.grain, "since": p.since, "until": p.until, "currency": p.currency,
	}[name]
	switch {
	case name == "since" || name == "until":
		return "DATE '" + text + "'"
	case text == "":
		return "CAST(NULL AS VARCHAR)"
	default:
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	}
}

// runRecipe runs the shipped recipe file with its params row replaced by p.
func runRecipe(t *testing.T, file string, p recipeParams) document.SQL {
	t.Helper()
	sql, err := spliceParams(repoFile(t, recipeDir+file), recipeParamNames[file], p)
	require.NoError(t, err, file)
	return runSQLText(t, sql)
}

// runShippedRecipe runs the recipe file exactly as shipped.
func runShippedRecipe(t *testing.T, file string) document.SQL {
	t.Helper()
	return runSQLText(t, repoFile(t, recipeDir+file))
}

// runSQLText runs `quarry sql --json -` with sql on stdin and decodes its document.
func runSQLText(t *testing.T, sql string) document.SQL {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := spendEnv(&stdout, &stderr)
	env.Stdin = strings.NewReader(sql)

	exitCode := runWith(context.Background(), []string{"sql", "--json", "-"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc document.SQL
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc
}

// recipeTotals sums the rows' last cell, an amount, as cents per value of the currency cell.
func recipeTotals(t *testing.T, rows [][]any, currencyCol int) map[string]int64 {
	t.Helper()
	totals := map[string]int64{}
	for _, row := range rows {
		totals[fmt.Sprint(row[currencyCol])] += centsOf(t, fmt.Sprint(row[len(row)-1]))
	}
	return totals
}

// spendTotalsByCurrency is a spend document's totals as cents per currency.
func spendTotalsByCurrency(t *testing.T, totals []spendMoney) map[string]int64 {
	t.Helper()
	sums := map[string]int64{}
	for _, total := range totals {
		sums[total.Currency] += centsOf(t, total.Spent)
	}
	return sums
}

// subtreeTotals sums the category rows named in categories as cents per currency.
func subtreeTotals(t *testing.T, rows []spendMoney, categories ...string) map[string]int64 {
	t.Helper()
	sums := map[string]int64{}
	for _, row := range rows {
		if row.Category != nil && slices.Contains(categories, *row.Category) {
			sums[row.Currency] += centsOf(t, row.Spent)
		}
	}
	return sums
}

// categoryNames lists the category of each row that has one.
func categoryNames(rows []spendMoney) []string {
	var names []string
	for _, row := range rows {
		if row.Category != nil {
			names = append(names, *row.Category)
		}
	}
	return names
}

// rowsOfYear is the rows whose first cell, a date, falls in year.
func rowsOfYear(rows [][]any, year string) [][]any {
	var inYear [][]any
	for _, row := range rows {
		if strings.HasPrefix(fmt.Sprint(row[0]), year) {
			inYear = append(inYear, row)
		}
	}
	return inYear
}

func Test_spending_trend_recipe_agrees_with_quarry_spend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillEvalStore(t, home)

	t.Run("no_filter_year", func(t *testing.T) {
		recipe := runRecipe(t, spendingTrendFile, recipeParams{grain: "year", since: "2022-01-01", until: "2026-12-31", currency: "CAD"})
		wantCurrencies := map[string][]string{
			"2022": {"CAD"}, "2023": {"CAD"}, "2024": {"CAD"}, "2025": {"CAD", "USD"}, "2026": {"CAD", "USD"},
		}

		for year, currencies := range wantCurrencies {
			t.Run(year, func(t *testing.T) {
				command, _ := runSpendJSON(t, "--since", year, "--until", year)

				require.Equal(t, "CAD", command.Currency)
				commandTotals := spendTotalsByCurrency(t, command.Totals)
				assert.Equal(t, currencies, slices.Sorted(maps.Keys(commandTotals)))
				assert.Equal(t, commandTotals, recipeTotals(t, rowsOfYear(recipe.Rows, year), 1))
			})
		}
	})

	t.Run("category_food", func(t *testing.T) {
		recipe := runRecipe(t, spendingTrendFile, recipeParams{category: "food", grain: "year", since: "2026-01-01", until: "2026-12-31", currency: "CAD"})

		command, _ := runSpendJSON(t, "--by", "category", "--since", "2026", "--until", "2026")

		require.Equal(t, "CAD", command.Currency)
		want := subtreeTotals(t, command.Rows, "Food", "Food:Groceries", "Food:Groceries:Organic")
		assert.NotEmpty(t, want)
		assert.Contains(t, categoryNames(command.Rows), "Foodies")
		assert.Equal(t, want, recipeTotals(t, recipe.Rows, 1))
	})

	t.Run("shipped_values", func(t *testing.T) {
		recipe := runShippedRecipe(t, spendingTrendFile)

		assert.Equal(t, [][]any{
			{"2022-01-01", "CAD", "22.50"},
			{"2023-01-01", "CAD", "33.75"},
			{"2024-01-01", "CAD", "56.25"},
			{"2025-01-01", "CAD", "255.50"},
			{"2025-01-01", "USD", "21.98"},
			{"2026-01-01", "CAD", "402.79"},
			{"2026-01-01", "USD", "21.98"},
		}, recipe.Rows)
	})

	t.Run("pre_rate_split_on_native_row", func(t *testing.T) {
		recipe := runRecipe(t, spendingTrendFile, recipeParams{payee: "Spotify", grain: "month", since: "2026-02-01", until: "2026-03-31", currency: "CAD"})

		assert.Equal(t, [][]any{{"2026-02-01", "USD", "10.99"}, {"2026-03-01", "CAD", "14.29"}}, recipe.Rows)
	})
}

func Test_income_by_category_recipe_agrees_with_quarry_cashflow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillEvalStore(t, home)

	recipe := runRecipe(t, incomeByCatFile, recipeParams{since: "2026-01-01", until: "2026-12-31", currency: "CAD"})
	command, _ := runCashFlowJSON(t, "--by", "year", "--since", "2026", "--until", "2026")

	require.Equal(t, "CAD", command.Currency)
	wantIncome := map[string]int64{}
	for _, total := range command.Totals {
		wantIncome[total.Currency] += centsOf(t, total.Income)
	}
	assert.Equal(t, []string{"CAD", "USD"}, slices.Sorted(maps.Keys(wantIncome)))
	assert.Equal(t, wantIncome, recipeTotals(t, recipe.Rows, 1))
	assert.Equal(t, [][]any{
		{"(uncategorized)", "CAD", "128.00"}, {"Income:Salary", "CAD", "10130.00"}, {"Income:Salary", "USD", "1000.00"},
	}, recipe.Rows)
}

func Test_recipe_params_line_is_found_once_and_keeps_its_names(t *testing.T) {
	names := recipeParamNames[incomeByCatFile]
	line := recipeParamsPrefix + "DATE '2020-01-01' AS since, current_date AS until, 'CAD' AS currency)"
	params := recipeParams{since: "2026-01-01", until: "2026-12-31", currency: "CAD"}

	cases := []struct {
		name, sql string
		err       error
	}{
		{"one_line", "-- q\n" + line + "\nSELECT 1", nil},
		{"no_line", "-- q\nSELECT 1", errNoParamsLine},
		{"two_lines", line + "\n" + line, errTwoParamsLines},
		{"other_order", recipeParamsPrefix + "'CAD' AS currency, DATE '2020-01-01' AS since, current_date AS until)", errParamAliases},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := spliceParams(c.sql, names, params)

			if c.err != nil {
				assert.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "-- q\n"+recipeParamsPrefix+"DATE '2026-01-01' AS since, DATE '2026-12-31' AS until, 'CAD' AS currency)\nSELECT 1", got)
		})
	}
}

// recipeScenario sets HOME to a fresh skillEvalStore.
func recipeScenario(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillEvalStore(t, home)
}

func Test_spending_trend_category_matches_the_subtree_ignoring_case(t *testing.T) {
	recipeScenario(t)
	command, _ := runSpendJSON(t, "--by", "category", "--since", "2026", "--until", "2026")
	require.Equal(t, "CAD", command.Currency)
	require.Contains(t, categoryNames(command.Rows), "Foodies")
	cases := []struct {
		name, category string
		subtree        []string
	}{
		{"upper_case_root", "FOOD", []string{"Food", "Food:Groceries", "Food:Groceries:Organic"}},
		{"lower_case_child", "food:groceries", []string{"Food:Groceries", "Food:Groceries:Organic"}},
		{"lower_case_leaf", "food:groceries:organic", []string{"Food:Groceries:Organic"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			recipe := runRecipe(t, spendingTrendFile, recipeParams{category: c.category, grain: "year", since: "2026-01-01", until: "2026-12-31", currency: "CAD"})

			want := subtreeTotals(t, command.Rows, c.subtree...)
			assert.NotEmpty(t, want)
			assert.Equal(t, want, recipeTotals(t, recipe.Rows, 1))
		})
	}
}

func Test_spending_trend_payee_matches_the_exact_name_ignoring_case(t *testing.T) {
	recipeScenario(t)
	hardware := [][]any{{"2025-01-01", "CAD", "200.00"}, {"2026-01-01", "CAD", "250.00"}}
	cases := []struct {
		name, payee, category string
		want                  [][]any
	}{
		{"exact", "Hardware", "", hardware},
		{"lower_case", "hardware", "", hardware},
		{"upper_case", "HARDWARE", "", hardware},
		{"prefix_only", "Hard", "", [][]any{}},
		{"other_category", "Hardware", "Auto", [][]any{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			recipe := runRecipe(t, spendingTrendFile, recipeParams{category: c.category, payee: c.payee, grain: "year", since: "2025-01-01", until: "2026-12-31", currency: "CAD"})

			assert.Equal(t, c.want, recipe.Rows)
		})
	}
}

// spendByMonth is the 2026 rows of spend --by month that spent anything, as cents keyed "YYYY-MM|currency".
func spendByMonth(t *testing.T) map[string]int64 {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), []string{"spend", "--json", "--by", "month", "--since", "2026", "--until", "2026"}, spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Rows []struct {
			Month    string `json:"month"`
			Currency string `json:"currency"`
			Spent    string `json:"spent"`
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	months := map[string]int64{}
	for _, row := range doc.Rows {
		if cents := centsOf(t, row.Spent); cents != 0 {
			months[row.Month+"|"+row.Currency] += cents
		}
	}
	return months
}

func Test_spending_trend_grain_month_matches_spend_by_month(t *testing.T) {
	recipeScenario(t)

	recipe := runRecipe(t, spendingTrendFile, recipeParams{grain: "month", since: "2026-01-01", until: "2026-12-31", currency: "CAD"})

	got := map[string]int64{}
	for _, row := range recipe.Rows {
		got[fmt.Sprint(row[0])[:7]+"|"+fmt.Sprint(row[1])] += centsOf(t, fmt.Sprint(row[2]))
	}
	want := spendByMonth(t)
	assert.Contains(t, want, "2026-02|USD")
	assert.Contains(t, want, "2026-03|CAD")
	assert.Equal(t, want, got)
}

func Test_spending_trend_currency_usd_matches_spend_currency_usd(t *testing.T) {
	recipeScenario(t)
	command, _ := runSpendJSON(t, "--currency", "USD", "--since", "2026", "--until", "2026")

	recipe := runRecipe(t, spendingTrendFile, recipeParams{grain: "year", since: "2026-01-01", until: "2026-12-31", currency: "USD"})

	require.Equal(t, "USD", command.Currency)
	want := spendTotalsByCurrency(t, command.Totals)
	assert.Equal(t, []string{"CAD", "USD"}, slices.Sorted(maps.Keys(want)))
	assert.Equal(t, want, recipeTotals(t, recipe.Rows, 1))
}

func Test_spending_trend_since_and_until_include_both_ends_only(t *testing.T) {
	recipeScenario(t)

	recipe := runRecipe(t, spendingTrendFile, recipeParams{grain: "year", since: "2024-06-10", until: "2024-06-20", currency: "CAD"})

	assert.Equal(t, [][]any{{"2024-01-01", "CAD", "6.00"}}, recipe.Rows)
}

func Test_spending_trend_lists_cad_before_the_first_rate_natively_in_usd_mode(t *testing.T) {
	recipeScenario(t)

	recipe := runRecipe(t, spendingTrendFile, recipeParams{payee: "Hardware", grain: "year", since: "2025-01-01", until: "2026-12-31", currency: "USD"})

	assert.Equal(t, [][]any{{"2025-01-01", "CAD", "200.00"}, {"2026-01-01", "USD", "192.31"}}, recipe.Rows)
}

func Test_spending_trend_leaves_out_the_transfer(t *testing.T) {
	recipeScenario(t)
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), []string{"search", "Savings Sweep", "--json"}, spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	found := decodeSearchJSON(t, stdout.String()).Transactions
	require.Len(t, found, 2)
	for _, txn := range found {
		require.True(t, txn.Transfer, txn.TransactionID)
	}

	recipe := runRecipe(t, spendingTrendFile, recipeParams{payee: "Savings Sweep", grain: "year", since: "2022-01-01", until: "2026-12-31", currency: "CAD"})

	assert.Empty(t, recipe.Rows)
}

func Test_spending_trend_columns_are_period_currency_spent(t *testing.T) {
	recipeScenario(t)

	recipe := runShippedRecipe(t, spendingTrendFile)

	assert.Equal(t, []document.SQLColumn{{Name: "period", Type: "DATE"}, {Name: "currency", Type: "VARCHAR"}, {Name: "spent", Type: "DECIMAL(18,2)"}}, recipe.Columns)
}

func Test_income_by_category_currency_usd_matches_cashflow_currency_usd(t *testing.T) {
	recipeScenario(t)
	command, _ := runCashFlowJSON(t, "--currency", "USD", "--by", "year", "--since", "2026", "--until", "2026")

	recipe := runRecipe(t, incomeByCatFile, recipeParams{since: "2026-01-01", until: "2026-12-31", currency: "USD"})

	require.Equal(t, "USD", command.Currency)
	want := map[string]int64{}
	for _, total := range command.Totals {
		want[total.Currency] += centsOf(t, total.Income)
	}
	assert.Equal(t, []string{"USD"}, slices.Sorted(maps.Keys(withoutZeros(want))))
	assert.Equal(t, withoutZeros(want), recipeTotals(t, recipe.Rows, 1))
}

// withoutZeros is totals less the currencies that summed to nothing; a report lists those, a query does not.
func withoutZeros(totals map[string]int64) map[string]int64 {
	kept := maps.Clone(totals)
	maps.DeleteFunc(kept, func(_ string, cents int64) bool { return cents == 0 })
	return kept
}

func Test_income_by_category_since_and_until_include_both_ends_only(t *testing.T) {
	recipeScenario(t)

	recipe := runRecipe(t, incomeByCatFile, recipeParams{since: "2024-06-10", until: "2024-06-20", currency: "CAD"})

	assert.Equal(t, [][]any{{"Income:Interest", "CAD", "6.00"}}, recipe.Rows)
}

func Test_income_by_category_lists_cad_before_the_first_rate_natively_in_usd_mode(t *testing.T) {
	recipeScenario(t)

	recipe := runRecipe(t, incomeByCatFile, recipeParams{since: "2024-01-01", until: "2024-12-31", currency: "USD"})

	assert.Equal(t, [][]any{{"Income:Interest", "CAD", "15.00"}}, recipe.Rows)
}

func Test_income_by_category_columns_are_category_currency_income(t *testing.T) {
	recipeScenario(t)

	recipe := runShippedRecipe(t, incomeByCatFile)

	assert.Equal(t, []document.SQLColumn{{Name: "category", Type: "VARCHAR"}, {Name: "currency", Type: "VARCHAR"}, {Name: "income", Type: "DECIMAL(18,2)"}}, recipe.Columns)
}

func Test_income_by_category_runs_as_shipped(t *testing.T) {
	recipeScenario(t)

	recipe := runShippedRecipe(t, incomeByCatFile)

	assert.Len(t, recipe.Columns, 3)
}

// recipeFiles are the shipped recipes and the one view each reads.
var recipeFiles = map[string]string{spendingTrendFile: "v_spending", incomeByCatFile: "v_cash_flow"}

// recipeLiterals are the quoted strings each recipe may hold off its params row: structural branch labels, not values.
var recipeLiterals = map[string][]string{
	spendingTrendFile: {"':'", "'CAD'"},
	incomeByCatFile:   {"'income'", "'CAD'", "'(uncategorized)'"},
}

var (
	zTableName   = regexp.MustCompile(`\bZ[A-Z]\w*`)
	likeOperator = regexp.MustCompile(`(?i)\bi?like\b`)
	clockCall    = regexp.MustCompile(`(?i)\b(?:now|today)\s*\(`)
	clockKeyword = regexp.MustCompile(`(?i)\bcurrent_(?:date|time|timestamp)\b`)
	quotedValue  = regexp.MustCompile(`'[^']*'`)
)

// linesWithout is sql less its lines that start with prefix.
func linesWithout(sql, prefix string) string {
	var kept []string
	for line := range strings.SplitSeq(sql, "\n") {
		if !strings.HasPrefix(line, prefix) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// relationsNamed is the relations that sql names as whole words outside its `--` comment lines.
func relationsNamed(sql string, relations []string) []string {
	code := linesWithout(sql, "--")
	var named []string
	for _, relation := range relations {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(relation) + `\b`).MatchString(code) {
			named = append(named, relation)
		}
	}
	return named
}

// forbiddenNames is what a recipe may not contain: a Quicken Z-table name, LIKE, or a clock read
// (current_date is allowed on the params row only).
func forbiddenNames(sql string) []string {
	var found []string
	for _, pattern := range []*regexp.Regexp{zTableName, likeOperator, clockCall} {
		found = append(found, pattern.FindAllString(sql, -1)...)
	}
	return append(found, clockKeyword.FindAllString(linesWithout(sql, recipeParamsPrefix), -1)...)
}

// quotedOffParamsRow is the quoted strings in sql outside its params row and comment lines.
func quotedOffParamsRow(sql string) []string {
	return quotedValue.FindAllString(linesWithout(linesWithout(sql, recipeParamsPrefix), "--"), -1)
}

// openingProblems names what is wrong with the first two lines of a recipe.
func openingProblems(sql string) []string {
	lines := strings.SplitN(sql, "\n", 3)
	var problems []string
	if !strings.HasPrefix(lines[0], "-- ") || !strings.HasSuffix(lines[0], "?") {
		problems = append(problems, "line 1 is not a `-- ` question")
	}
	if len(lines) < 2 || !strings.HasPrefix(lines[1], recipeParamsPrefix) {
		problems = append(problems, "line 2 is not the params row")
	}
	return problems
}

// storeRelations is the names of the tables and views of the store under a fresh HOME.
func storeRelations(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillEvalStore(t, home)
	schema, err := duckstore.New(storeDirUnder(home)).Schema(t.Context())
	require.NoError(t, err)
	names := make([]string, len(schema.Relations))
	for i, relation := range schema.Relations {
		names[i] = relation.Name
	}
	return names
}

func Test_recipes_read_only_their_view(t *testing.T) {
	relations := storeRelations(t)

	for file, view := range recipeFiles {
		t.Run(file, func(t *testing.T) {
			assert.Equal(t, []string{view}, relationsNamed(repoFile(t, recipeDir+file), relations))
		})
	}
}

func Test_recipes_name_no_quicken_table_like_or_clock(t *testing.T) {
	for file := range recipeFiles {
		t.Run(file, func(t *testing.T) {
			assert.Empty(t, forbiddenNames(repoFile(t, recipeDir+file)))
		})
	}
}

func Test_recipes_open_with_a_question_and_the_params_row(t *testing.T) {
	for file := range recipeFiles {
		t.Run(file, func(t *testing.T) {
			assert.Empty(t, openingProblems(repoFile(t, recipeDir+file)))
		})
	}
}

// shippedParamsLine is line 2 of each recipe: the values it ships with.
var shippedParamsLine = map[string]string{
	spendingTrendFile: recipeParamsPrefix + "'Food:Groceries' AS category, CAST(NULL AS VARCHAR) AS payee, 'year' AS grain, DATE '2022-01-01' AS since, current_date AS until, 'CAD' AS currency)",
	incomeByCatFile:   recipeParamsPrefix + "date_trunc('year', current_date) AS since, current_date AS until, 'CAD' AS currency)",
}

func Test_recipes_ship_the_ruled_params_line(t *testing.T) {
	for file, want := range shippedParamsLine {
		t.Run(file, func(t *testing.T) {
			lines := strings.SplitN(repoFile(t, recipeDir+file), "\n", 3)

			require.Greater(t, len(lines), 1)
			assert.Equal(t, want, lines[1])
		})
	}
}

func Test_recipes_put_values_only_in_the_params_row(t *testing.T) {
	for file, allowed := range recipeLiterals {
		t.Run(file, func(t *testing.T) {
			assert.Subset(t, allowed, quotedOffParamsRow(repoFile(t, recipeDir+file)))
		})
	}
}

func Test_recipe_scanners_flag_crafted_text(t *testing.T) {
	relations := storeRelations(t)
	cases := []struct {
		name string
		scan func(string) []string
		text string
		want []string
	}{
		{"table_outside_the_view", func(s string) []string { return relationsNamed(s, relations) }, "SELECT 1 FROM transactions t", []string{"transactions"}},
		{"table_named_only_in_a_comment", func(s string) []string { return relationsNamed(s, relations) }, "-- from transactions\nSELECT 1 FROM v_spending", []string{"v_spending"}},
		{"view_that_extends_a_relation_name", func(s string) []string { return relationsNamed(s, relations) }, "SELECT 1 FROM v_spending_extra", nil},
		{"quicken_table", forbiddenNames, "SELECT ZPAYEE FROM x", []string{"ZPAYEE"}},
		{"like", forbiddenNames, "WHERE a like 'x%'", []string{"like"}},
		{"ilike", forbiddenNames, "WHERE a ILIKE 'x%'", []string{"ILIKE"}},
		{"now", forbiddenNames, "SELECT now()", []string{"now("}},
		{"today", forbiddenNames, "SELECT today ()", []string{"today ("}},
		{"clock_keyword_off_the_params_row", forbiddenNames, recipeParamsPrefix + "current_date AS until)\nWHERE d < current_date", []string{"current_date"}},
		{"clock_keyword_on_the_params_row", forbiddenNames, recipeParamsPrefix + "current_date AS until)\nSELECT 1", nil},
		{"literal_off_the_params_row", quotedOffParamsRow, "SELECT 1 WHERE c = 'Food'", []string{"'Food'"}},
		{"literal_on_the_params_row", quotedOffParamsRow, recipeParamsPrefix + "'Food' AS category)\nSELECT 1", nil},
		{"literal_in_a_comment", quotedOffParamsRow, "-- what's 'Food'?\nSELECT 1", nil},
		{"first_line_not_a_comment", openingProblems, "SELECT 1\n" + recipeParamsPrefix, []string{"line 1 is not a `-- ` question"}},
		{"question_without_the_mark", openingProblems, "-- Why\n" + recipeParamsPrefix, []string{"line 1 is not a `-- ` question"}},
		{"comment_without_the_space", openingProblems, "--Why?\n" + recipeParamsPrefix, []string{"line 1 is not a `-- ` question"}},
		{"second_line_not_the_params_row", openingProblems, "-- Why?\nSELECT 1", []string{"line 2 is not the params row"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.scan(c.text))
		})
	}
}
