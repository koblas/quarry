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

// syncedHome syncs b under a fresh HOME and returns that HOME.
func syncedHome(t *testing.T, b *v9fixture.Builder) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncInvestmentFixture(t, home, b)
	return home
}

// investmentCash adds an investment transaction of action code with one entry in category (0 for none).
func investmentCash(b *v9fixture.Builder, day time.Time, code, category int64, row v9fixture.TransactionRow) {
	row.PostedDate = &day
	row.Type = &code
	b.Entry(v9fixture.EntryRow{Parent: b.InvestmentTransaction(row), Amount: row.Amount, CategoryTag: category})
}

// inAccount is a transaction row in the account with Z_PK accountPK carrying amount.
func inAccount(accountPK int64, amount string) v9fixture.TransactionRow {
	return v9fixture.TransactionRow{Account: accountPK, Amount: amount}
}

// incomeAndTradesFixture holds March investment income and buys and sells in Brokerage, plus a Chequing grocery split.
func incomeAndTradesFixture() *v9fixture.Builder {
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
	investmentCash(b, day, investmentCodeDividend, dividendsPK, inAccount(brokeragePK, "12.00"))
	investmentCash(b, day, investmentCodeInterest, interestPK, inAccount(brokeragePK, "3.00"))
	investmentCash(b, day, investmentCodeCapitalGain, gainsPK, v9fixture.TransactionRow{Account: brokeragePK, Position: positionPK, Units: "0", Amount: "4.00"})
	investmentCash(b, day, investmentCodeBuy, tradesPK, v9fixture.TransactionRow{Account: brokeragePK, Position: positionPK, Units: "10", Amount: "-1000.50"})
	investmentCash(b, day, investmentCodeSell, tradesPK, v9fixture.TransactionRow{Account: brokeragePK, Position: positionPK, Units: "-4", Amount: "400.25"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "6"})
	registerPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-20.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: registerPK, Amount: "-20.00", CategoryTag: groceriesPK})
	return b
}

// marginInterestFixture holds one March margin-interest charge and a system-category buy in Brokerage.
func marginInterestFixture() *v9fixture.Builder {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	marginPK := b.Category(v9fixture.TagRow{Name: "Margin Interest", Type: new(int64(categoryKindExpense))})
	tradesPK := b.Category(v9fixture.TagRow{Name: "Trades", Type: new(int64(categoryKindSystem))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investmentCash(b, day, investmentCodeMarginInterest, marginPK, inAccount(brokeragePK, "-7.25"))
	investmentCash(b, day, investmentCodeBuy, tradesPK, v9fixture.TransactionRow{Account: brokeragePK, Position: positionPK, Units: "5", Amount: "-500.00"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "5"})
	return b
}

// cashFlowByCategory is v_cash_flow's total per category and flow, as "flow|amount".
func cashFlowByCategory(t *testing.T, home string) map[string]string {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return stringMap(t, db, `SELECT category, flow || '|' || CAST(SUM(amount) AS VARCHAR) FROM v_cash_flow GROUP BY category, flow`)
}

func Test_run_cashflow_counts_investment_dividends_interest_and_capital_gains_as_income_and_buys_and_sells_as_neither(t *testing.T) {
	home := syncedHome(t, incomeAndTradesFixture())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent    Net  Savings rate  Status\n"+
		"2026-03  CAD        19.00  20.00  -1.00         -5.3%\n"+
		"Total    CAD        19.00  20.00  -1.00         -5.3%\n",
		stdout.String())
	assert.Equal(t, map[string]string{
		"Dividends":     "income|12.00",
		"Interest":      "income|3.00",
		"Capital Gains": "income|4.00",
		"Groceries":     "expense|-20.00",
	}, cashFlowByCategory(t, home),
		"the buy and the sell, in a system category, are not in v_cash_flow")
}

func Test_run_cashflow_for_the_brokerage_account_reports_its_investment_income(t *testing.T) {
	syncedHome(t, incomeAndTradesFixture())
	args := []string{"--account", "Brokerage", "--since", "2026-03", "--until", "2026-03"}

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), append([]string{"cashflow"}, args...), &stdout, &stderr)

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in Brokerage, amounts in CAD\n\n"+
			"Month    Currency  Income  Spent    Net  Savings rate  Status\n"+
			"2026-03  CAD        19.00   0.00  19.00        100.0%\n"+
			"Total    CAD        19.00   0.00  19.00        100.0%\n",
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		doc, _ := runCashFlowJSON(t, args...)

		require.Len(t, doc.Totals, 1)
		assert.Equal(t, "19.00", doc.Totals[0].Income)
		assert.Equal(t, "0.00", doc.Totals[0].Spent)
		assert.Equal(t, "19.00", doc.Totals[0].Net)
	})
}

func Test_run_spend_counts_investment_margin_interest_as_spending(t *testing.T) {
	syncedHome(t, marginInterestFixture())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"spend", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	const row = "%-15s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Margin Interest", "CAD", "7.25")+
		fmt.Sprintf(row, "Total", "CAD", "7.25"),
		stdout.String(),
		"the buy, in a system category, is not spending")
}

func Test_run_spend_for_the_brokerage_account_reports_its_margin_interest(t *testing.T) {
	syncedHome(t, marginInterestFixture())
	args := []string{"--account", "Brokerage", "--since", "2026-03", "--until", "2026-03"}

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), append([]string{"spend"}, args...), &stdout, &stderr)

		require.Equal(t, 0, exitCode, stderr.String())
		const row = "%-15s  %-8s  %5s\n"
		assert.Equal(t, "Spending 2026-03-01 to 2026-03-31 in Brokerage, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Margin Interest", "CAD", "7.25")+
			fmt.Sprintf(row, "Total", "CAD", "7.25"),
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		doc, _ := runSpendJSON(t, args...)

		require.Len(t, doc.Rows, 1)
		require.NotNil(t, doc.Rows[0].Category)
		assert.Equal(t, "Margin Interest", *doc.Rows[0].Category)
		assert.Equal(t, "7.25", doc.Rows[0].Spent)
		require.Len(t, doc.Totals, 1)
		assert.Equal(t, "7.25", doc.Totals[0].Spent)
	})
}

func Test_run_cashflow_counts_investment_income_and_margin_interest_in_a_retirement_account(t *testing.T) {
	b := v9fixture.NewBuilder()
	retirementPK := b.Account(v9fixture.AccountRow{Name: "Retirement", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
	dividendsPK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	marginPK := b.Category(v9fixture.TagRow{Name: "Margin Interest", Type: new(int64(categoryKindExpense))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investmentCash(b, day, investmentCodeDividend, dividendsPK, inAccount(retirementPK, "6.00"))
	investmentCash(b, day, investmentCodeMarginInterest, marginPK, inAccount(retirementPK, "-2.00"))
	syncedHome(t, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--account", "Retirement", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in Retirement, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026-03  CAD         6.00   2.00  4.00         66.7%\n"+
		"Total    CAD         6.00   2.00  4.00         66.7%\n",
		stdout.String())
}

func Test_run_cashflow_classifies_miscellaneous_investment_cash_by_its_category_not_its_action(t *testing.T) {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	rebatesPK := b.Category(v9fixture.TagRow{Name: "Rebates", Type: new(int64(categoryKindIncome))})
	feesPK := b.Category(v9fixture.TagRow{Name: "Account Fees", Type: new(int64(categoryKindExpense))})
	adjustmentsPK := b.Category(v9fixture.TagRow{Name: "Adjustments", Type: new(int64(categoryKindSystem))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investmentCash(b, day, investmentCodeMiscIncome, rebatesPK, inAccount(brokeragePK, "9.00"))
	investmentCash(b, day, investmentCodeMiscIncome, adjustmentsPK, inAccount(brokeragePK, "4.00"))
	investmentCash(b, day, investmentCodeMiscExpense, feesPK, inAccount(brokeragePK, "-5.00"))
	investmentCash(b, day, investmentCodeMiscExpense, adjustmentsPK, inAccount(brokeragePK, "-3.00"))
	home := syncedHome(t, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026-03  CAD         9.00   5.00  4.00         44.4%\n"+
		"Total    CAD         9.00   5.00  4.00         44.4%\n",
		stdout.String())
	assert.Equal(t, map[string]string{
		"Rebates":      "income|9.00",
		"Account Fees": "expense|-5.00",
	}, cashFlowByCategory(t, home),
		"miscellaneous cash in a system category is in neither flow")
}

func Test_run_cashflow_counts_an_uncategorized_investment_transaction_by_its_sign(t *testing.T) {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investmentCash(b, day, investmentCodeMiscIncome, 0, inAccount(brokeragePK, "9.00"))
	investmentCash(b, day, investmentCodeMiscIncome, 0, inAccount(brokeragePK, "2.50"))
	investmentCash(b, day, investmentCodeMiscExpense, 0, inAccount(brokeragePK, "-5.00"))
	syncedHome(t, b)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026-03  CAD        11.50   5.00  6.50         56.5%\n"+
		"Total    CAD        11.50   5.00  6.50         56.5%\n",
		stdout.String())
}
