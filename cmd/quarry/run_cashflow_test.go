// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cashFlowRows is spendRows plus an income category, cat-salary.
func cashFlowRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := spendRows(accounts, splits...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	return rows
}

// cashFlowLine is one cash-flow table line with every column but the last as wide as the fixtures' widest cells.
func cashFlowLine(periodWidth int, period, currency, income, spent, net, rate, status string) string {
	line := fmt.Sprintf("%-*s  %-8s  %9s  %9s  %9s  %12s", periodWidth, period, currency, income, spent, net, rate)
	if status != "" {
		line += "  " + status
	}
	return line + "\n"
}

func Test_run_cashflow_shows_income_spending_and_savings_rate_by_month(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{{ID: "acct-cad", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true}},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 910000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -300000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 1, 20), cents: -320000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 2, 28), cents: 500000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 4, 15), cents: -100000},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 9, 15), cents: 602000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 9, 20), cents: -511040},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"cashflow"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const w = 7
	assert.Equal(t, "Cash flow 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		cashFlowLine(w, "Month", "Currency", "Income", "Spent", "Net", "Savings rate", "Status")+
		cashFlowLine(w, "2026-01", "CAD", "9,100.00", "6,200.00", "2,900.00", "31.9%", "")+
		cashFlowLine(w, "2026-02", "CAD", "5,000.00", "0.00", "5,000.00", "100.0%", "")+
		cashFlowLine(w, "2026-03", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-04", "CAD", "0.00", "1,000.00", "-1,000.00", "n/a", "")+
		cashFlowLine(w, "2026-05", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-06", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-07", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-08", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-09", "CAD", "6,020.00", "5,110.40", "909.60", "15.1%", "partial")+
		cashFlowLine(w, "Total", "CAD", "20,120.00", "12,310.40", "7,809.60", "38.8%", ""),
		stdout.String())
}

func Test_run_cashflow_by_year_shows_one_row_per_year_and_na_without_income(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{{ID: "acct-cad", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true}},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2020, 6, 1), cents: 1000000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2020, 7, 1), cents: -400000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2021, 3, 1), cents: 300000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2022, 5, 1), cents: -50000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2024, 2, 1), cents: 100000},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2024, 8, 1), cents: -200000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2025, 4, 1), cents: 800000},
		spendSplit{id: "s08", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 11, 1), cents: -600000},
		spendSplit{id: "s09", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 5), cents: 999900},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(),
		[]string{"cashflow", "--by", "year", "--since", "2020", "--until", "2025"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const w = 5
	assert.Equal(t, "Cash flow 2020-01-01 to 2025-12-31 in all accounts\n\n"+
		cashFlowLine(w, "Year", "Currency", "Income", "Spent", "Net", "Savings rate", "Status")+
		cashFlowLine(w, "2020", "CAD", "10,000.00", "4,000.00", "6,000.00", "60.0%", "")+
		cashFlowLine(w, "2021", "CAD", "3,000.00", "0.00", "3,000.00", "100.0%", "")+
		cashFlowLine(w, "2022", "CAD", "0.00", "500.00", "-500.00", "n/a", "")+
		cashFlowLine(w, "2023", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2024", "CAD", "1,000.00", "2,000.00", "-1,000.00", "-100.0%", "")+
		cashFlowLine(w, "2025", "CAD", "8,000.00", "6,000.00", "2,000.00", "25.0%", "")+
		cashFlowLine(w, "Total", "CAD", "22,000.00", "12,500.00", "9,500.00", "43.2%", ""),
		stdout.String())
}
