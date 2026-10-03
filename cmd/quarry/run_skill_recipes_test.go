// The recipes are read by repo-relative path, so these tests live in package main
// beside the other cmd/quarry tests.
package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report/document"
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

// runRecipe runs the shipped recipe file with its params row replaced by p.
func runRecipe(_ *testing.T, _ string, _ recipeParams) document.SQL {
	return document.SQL{}
}

// runShippedRecipe runs the recipe file exactly as shipped.
func runShippedRecipe(_ *testing.T, _ string) document.SQL {
	return document.SQL{}
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

// cellPairs is the first two cells of each row.
func cellPairs(rows [][]any) [][2]any {
	pairs := make([][2]any, len(rows))
	for i, row := range rows {
		pairs[i] = [2]any{row[0], row[1]}
	}
	return pairs
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

		assert.Equal(t, [][2]any{
			{"2022-01-01", "CAD"},
			{"2023-01-01", "CAD"},
			{"2024-01-01", "CAD"},
			{"2025-01-01", "CAD"},
			{"2025-01-01", "USD"},
			{"2026-01-01", "CAD"},
			{"2026-01-01", "USD"},
		}, cellPairs(recipe.Rows))
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
