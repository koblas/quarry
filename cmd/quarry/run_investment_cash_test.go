package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	investmentCodeAddShares  = 2
	investmentCodeBuy        = 3
	investmentCodeDividend   = 10
	investmentCodeMiscIncome = 12
	investmentCodeReinvest   = 15
	investmentCodeRemove     = 17
	investmentCodeSell       = 19
	investmentCodeSplit      = 23

	categoryKindIncome = 2
	categoryKindSystem = 0
)

func Test_run_sync_gives_each_investment_transaction_that_moves_cash_a_row_in_transactions(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	dividendsPK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	tradesPK := b.Category(v9fixture.TagRow{Name: "Trades", Type: new(int64(categoryKindSystem))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	type investment struct{ txn, entry int64 }
	invest := func(code int64, category int64, row v9fixture.TransactionRow) investment {
		row.Account = brokeragePK
		row.PostedDate = &day
		row.Type = &code
		pk := b.InvestmentTransaction(row)
		return investment{txn: pk, entry: b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount, CategoryTag: category})}
	}
	buy := invest(investmentCodeBuy, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "10", Amount: "-1000.50"})
	dividend := invest(investmentCodeDividend, dividendsPK, v9fixture.TransactionRow{Amount: "12.00"})
	invest(investmentCodeAddShares, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0.00"})
	invest(investmentCodeRemove, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0.00"})
	invest(investmentCodeReinvest, dividendsPK, v9fixture.TransactionRow{Position: positionPK, Units: "0.5", Amount: "0.00"})
	invest(investmentCodeSplit, tradesPK, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0.00", Numerator: "1", Denominator: "12"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "0.875"})
	registerPK := b.Transaction(v9fixture.TransactionRow{Account: brokeragePK, Amount: "-20.00", PostedDate: &day})
	registerEntry := b.Entry(v9fixture.EntryRow{Parent: registerPK, Amount: "-20.00", CategoryTag: tradesPK})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	txnID := func(pk int64) string { return fmt.Sprintf("txn-%d", pk) }
	itxnID := func(pk int64) string { return fmt.Sprintf("itxn-%d", pk) }
	splitID := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }
	assert.Equal(t, map[string]string{
		txnID(buy.txn):      "-1000.50|" + itxnID(buy.txn),
		txnID(dividend.txn): "12.00|" + itxnID(dividend.txn),
		txnID(registerPK):   "-20.00|NULL",
	}, stringMap(t, db, `SELECT id, CAST(amount AS VARCHAR) || '|' || COALESCE(investment_transaction_id, 'NULL') FROM transactions`),
		"one row per investment transaction with a non-zero amount, none for share-only actions or the zero-amount reinvest")
	assert.Equal(t, map[string]string{
		splitID(buy.entry):      txnID(buy.txn) + "|Trades|-1000.50",
		splitID(dividend.entry): txnID(dividend.txn) + "|Dividends|12.00",
		splitID(registerEntry):  txnID(registerPK) + "|Trades|-20.00",
	}, stringMap(t, db, `SELECT s.id, s.transaction_id || '|' || c.full_path || '|' || CAST(s.amount AS VARCHAR)
		FROM splits s JOIN categories c ON c.id = s.category_id`))
	assert.Equal(t, map[string]string{"cash": "-1008.50"},
		stringMap(t, db, `SELECT 'cash', CAST(SUM(amount) AS VARCHAR) FROM transactions WHERE account_id = 'acct-`+strconv.FormatInt(brokeragePK, 10)+`'`))
	assert.Equal(t, []string{"9"}, storeTextRows(t, home, "SELECT CAST(format_version AS VARCHAR) FROM store_info"))
}

func Test_run_sync_gives_a_reinvested_dividend_no_row_and_no_income(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	dividendsPK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	invest := func(code int64, row v9fixture.TransactionRow) int64 {
		row.Account = brokeragePK
		row.PostedDate = &day
		row.Type = &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount, CategoryTag: dividendsPK})
		return pk
	}
	dividendPK := invest(investmentCodeDividend, v9fixture.TransactionRow{Amount: "12.00"})
	invest(investmentCodeReinvest, v9fixture.TransactionRow{Position: positionPK, Units: "0.5", Amount: "0.00"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "0.5"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{fmt.Sprintf("txn-%d", dividendPK): "12.00"},
		stringMap(t, db, `SELECT id, CAST(amount AS VARCHAR) FROM transactions`))
	assert.Equal(t, map[string]string{"Dividends": "12.00"},
		stringMap(t, db, `SELECT category, CAST(SUM(amount) AS VARCHAR) FROM v_cash_flow WHERE flow = 'income' GROUP BY category`))
}

func Test_run_sync_pairs_an_investment_transfer_entry_and_gives_an_entry_less_investment_one_uncategorized_split(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	inReports := new(int64(1))
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true, UsedInReports: inReports})
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: inReports})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	later := day.AddDate(0, 0, 20)
	dividendCode, miscIncomeCode := int64(investmentCodeDividend), int64(investmentCodeMiscIncome)

	dividendPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &dividendCode, Amount: "12.00", PostedDate: &day})
	dividendLeg := b.Entry(v9fixture.EntryRow{Parent: dividendPK, Amount: "12.00", QuickenID: 1001, Transfer: "2002"})
	chequingTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-12.00", PostedDate: &day})
	chequingLeg := b.Entry(v9fixture.EntryRow{Parent: chequingTxn, Amount: "-12.00", QuickenID: 2002, Transfer: "1001"})
	miscPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &miscIncomeCode, Amount: "5.00", PostedDate: &later})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	splitID := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }
	acctID := func(pk int64) string { return fmt.Sprintf("acct-%d", pk) }
	syntheticID := fmt.Sprintf("split-itxn-%d", miscPK)
	assert.Equal(t, map[string]string{
		fmt.Sprintf("xfer-%d", dividendLeg): splitID(dividendLeg) + "|" + splitID(chequingLeg),
	}, stringMap(t, db, "SELECT id, from_split_id || '|' || to_split_id FROM transfers"), "one paired transfer between the dividend entry and the chequing entry")
	assert.Equal(t, map[string]string{
		splitID(dividendLeg): acctID(chequingPK),
		splitID(chequingLeg): acctID(brokeragePK),
		syntheticID:          "NULL",
	}, stringMap(t, db, "SELECT id, COALESCE(transfer_account_id, 'NULL') FROM splits"))
	assert.Equal(t, map[string]string{syntheticID: "income|5.00|NULL"},
		stringMap(t, db, "SELECT split_id, flow || '|' || CAST(amount AS VARCHAR) || '|' || COALESCE(category_id, 'NULL') FROM v_cash_flow"),
		"only the entry-less row's split is cash flow; the transfer pair is not")
	assert.Equal(t, []string{"uncategorized"}, storeTextRows(t, home, "SELECT type FROM findings"))
	assert.Equal(t, []string{syntheticID}, storeTextRows(t, home,
		"SELECT split_id FROM finding_items WHERE finding_id IN (SELECT id FROM findings WHERE type = 'uncategorized')"))
}

func Test_run_sync_twice_reproduces_investment_cash_ids_and_keeps_an_ignored_uncategorized_finding_ignored(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	dividendCode, miscIncomeCode := int64(investmentCodeDividend), int64(investmentCodeMiscIncome)
	dividendPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &dividendCode, Amount: "12.00", PostedDate: &day})
	dividendEntry := b.Entry(v9fixture.EntryRow{Parent: dividendPK, Amount: "12.00"})
	miscPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &miscIncomeCode, Amount: "5.00", PostedDate: &day})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	findingID := storeTextRows(t, home, "SELECT id FROM findings WHERE type = 'uncategorized'")
	require.Len(t, findingID, 1, "one no-payee uncategorized finding covers both cash rows")
	editStore(t, home, "UPDATE findings SET first_found_at = TIMESTAMP '2026-03-01 00:00:00'")
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n[accounts]\nnon-registered = [\"acct-%d\"]\n", findingID[0], brokeragePK))

	syncBundle(t, bundle)

	wantTxns := []string{fmt.Sprintf("txn-%d", dividendPK), fmt.Sprintf("txn-%d", miscPK)}
	wantSplits := []string{fmt.Sprintf("split-%d", dividendEntry), fmt.Sprintf("split-itxn-%d", miscPK)}
	slices.Sort(wantTxns)
	slices.Sort(wantSplits)
	assert.Equal(t, wantTxns, storeTextRows(t, home, "SELECT id FROM transactions ORDER BY id"))
	assert.Equal(t, wantSplits, storeTextRows(t, home, "SELECT id FROM splits ORDER BY id"))
	assert.Equal(t, []string{findingID[0] + "|2026-03-01 00:00:00"},
		storeTextRows(t, home, "SELECT id || '|' || CAST(first_found_at AS VARCHAR) FROM findings WHERE type = 'uncategorized'"))
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"findings"}, &stdout, &stderr), stderr.String())
	assert.Equal(t, "No open findings; 1 ignored not shown (--status all)\n", stdout.String())
}

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

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
}

// syncedHome syncs b under a fresh HOME and returns that HOME.
func syncedHome(t *testing.T, b *v9fixture.Builder) string {
	t.Helper()
	home := newHome(t)
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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"})

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
		exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"cashflow"}, args...))

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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"spend", "--since", "2026-03", "--until", "2026-03"})

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
		exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"spend"}, args...))

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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"cashflow", "--account", "Retirement", "--since", "2026-03", "--until", "2026-03"})

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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"})

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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Cash flow 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026-03  CAD        11.50   5.00  6.50         56.5%\n"+
		"Total    CAD        11.50   5.00  6.50         56.5%\n",
		stdout.String())
}

// monthlyOn12th is the 12th of the month i months after October 2025.
func monthlyOn12th(i int) time.Time {
	return time.Date(2025, time.October+time.Month(i), 12, 0, 0, 0, 0, time.UTC)
}

// marginInterestChargesFixture's Brokerage holds one margin-interest charge per amount, dated by when(i).
func marginInterestChargesFixture(when func(i int) time.Time, amounts ...string) *v9fixture.Builder {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	marginPK := b.Category(v9fixture.TagRow{Name: "Margin Interest", Type: new(int64(categoryKindExpense))})
	for i, amount := range amounts {
		investmentCash(b, when(i), investmentCodeMarginInterest, marginPK, inAccount(brokeragePK, amount))
	}
	return b
}

func Test_run_anomalies_lists_a_margin_interest_charge_against_its_categorys_earlier_charges(t *testing.T) {
	amounts := []string{"-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-150.00"}
	b := marginInterestChargesFixture(func(i int) time.Time {
		if i < 10 {
			return time.Date(2025, time.January+time.Month(i), 3, 0, 0, 0, 0, time.UTC)
		}
		return time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)
	}, amounts...)
	syncedHome(t, b)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
		[]string{"2026-03-02", "Brokerage (CAD)", "(no payee)", "Margin Interest", "150.00", "20.00", "7.5x", "category, 10 earlier"}),
		stdout.String())
}

func Test_run_anomalies_counts_a_margin_interest_charge_of_100_or_more_with_no_history_as_not_judged(t *testing.T) {
	b := marginInterestChargesFixture(func(i int) time.Time {
		return time.Date(2026, time.March, 1+i, 0, 0, 0, 0, time.UTC)
	}, "-20.00", "-30.00", "-150.00")
	syncedHome(t, b)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		"3 charges checked; 1 had too little history to judge"), stdout.String())
}

// marginInterestAndNetflixFixture is twelve monthly margin-interest charges in Brokerage, with no payee,
// beside twelve monthly Netflix.com charges in Chequing.
func marginInterestAndNetflixFixture() *v9fixture.Builder {
	amounts := make([]string, 12)
	for i := range amounts {
		amounts[i] = "-20.00"
	}
	b := marginInterestChargesFixture(monthlyOn12th, amounts...)
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	netflixPK := b.Payee(v9fixture.PayeeRow{Name: "Netflix.com"})
	subscriptionsPK := b.Category(v9fixture.TagRow{Name: "Subscriptions", Type: new(int64(categoryKindExpense))})
	for i := range 12 {
		day := monthlyOn12th(i)
		pk := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-20.99", PostedDate: &day, Payee: netflixPK})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "-20.99", CategoryTag: subscriptionsPK})
	}
	return b
}

func Test_run_recurring_finds_no_series_in_investment_rows_that_have_no_payee(t *testing.T) {
	syncedHome(t, marginInterestAndNetflixFixture())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_spend_by_payee_puts_margin_interest_in_the_no_payee_bucket(t *testing.T) {
	syncedHome(t, marginInterestAndNetflixFixture())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--by", "payee"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-11s  %-8s  %6s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Payee", "Currency", "Spent")+
		fmt.Sprintf(row, "Netflix.com", "CAD", "188.91")+
		fmt.Sprintf(row, "(no payee)", "CAD", "180.00")+
		fmt.Sprintf(row, "Total", "CAD", "368.91"),
		stdout.String())
}
