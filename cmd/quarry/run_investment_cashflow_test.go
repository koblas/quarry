package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	investmentCodeMarginInterest = 6
	investmentCodeMiscExpense    = 7
	investmentCodeCapitalGain    = 8
	investmentCodeInterest       = 11
	categoryKindExpense          = 1
)

// syncInvestmentFixture writes b's bundle under home and syncs it.
func syncInvestmentFixture(t *testing.T, home string, b *v9fixture.Builder) {
	t.Helper()
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
}

func Test_run_cashflow_counts_investment_dividends_interest_and_capital_gains_as_income_and_buys_and_sells_as_neither(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	dividendsPK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	interestPK := b.Category(v9fixture.TagRow{Name: "Interest", Type: new(int64(categoryKindIncome))})
	gainsPK := b.Category(v9fixture.TagRow{Name: "Capital Gains", Type: new(int64(categoryKindIncome))})
	tradesPK := b.Category(v9fixture.TagRow{Name: "Trades", Type: new(int64(categoryKindSystem))})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(categoryKindExpense))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	invest := func(code, category int64, row v9fixture.TransactionRow) {
		row.Account = brokeragePK
		row.PostedDate = &day
		row.Type = &code
		b.Entry(v9fixture.EntryRow{Parent: b.InvestmentTransaction(row), Amount: row.Amount, CategoryTag: category})
	}
	invest(investmentCodeDividend, dividendsPK, v9fixture.TransactionRow{Amount: "12.00"})
	invest(investmentCodeInterest, interestPK, v9fixture.TransactionRow{Amount: "3.00"})
	invest(investmentCodeCapitalGain, gainsPK, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "4.00"})
	invest(investmentCodeBuy, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "10", Amount: "-1000.50"})
	invest(investmentCodeSell, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "-4", Amount: "400.25"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "6"})
	registerPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-20.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: registerPK, Amount: "-20.00", CategoryTag: groceriesPK})
	syncInvestmentFixture(t, home, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent    Net  Savings rate  Status\n"+
		"2026-03  CAD        19.00  20.00  -1.00         -5.3%\n"+
		"Total    CAD        19.00  20.00  -1.00         -5.3%\n",
		stdout.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		"Dividends":     "income|12.00",
		"Interest":      "income|3.00",
		"Capital Gains": "income|4.00",
		"Groceries":     "expense|-20.00",
	}, stringMap(t, db, `SELECT category, flow || '|' || CAST(SUM(amount) AS VARCHAR) FROM v_cash_flow GROUP BY category, flow`),
		"the buy and the sell, in a system category, are not in v_cash_flow")
}

func Test_run_spend_counts_investment_margin_interest_as_spending(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	marginPK := b.Category(v9fixture.TagRow{Name: "Margin Interest", Type: new(int64(categoryKindExpense))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	code := int64(investmentCodeMarginInterest)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &code, Amount: "-7.25", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "-7.25", CategoryTag: marginPK})
	syncInvestmentFixture(t, home, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"spend", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	const row = "%-15s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Margin Interest", "CAD", "7.25")+
		fmt.Sprintf(row, "Total", "CAD", "7.25"),
		stdout.String())
}

func Test_run_cashflow_counts_an_uncategorized_investment_transaction_by_its_sign(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	invest := func(code int64, amount string) {
		row := v9fixture.TransactionRow{Account: brokeragePK, Type: &code, Amount: amount, PostedDate: &day}
		b.Entry(v9fixture.EntryRow{Parent: b.InvestmentTransaction(row), Amount: amount})
	}
	invest(investmentCodeMiscIncome, "9.00")
	invest(investmentCodeMiscIncome, "2.50")
	invest(investmentCodeMiscExpense, "-5.00")
	syncInvestmentFixture(t, home, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026-03  CAD        11.50   5.00  6.50         56.5%\n"+
		"Total    CAD        11.50   5.00  6.50         56.5%\n",
		stdout.String())
}
