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

func Test_run_sync_imports_securities_and_their_prices(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Ticker: "", Currency: "USD"})
	noCurrencyPK := b.Security(v9fixture.SecurityRow{Name: "Plain Co", Ticker: "PLN"})

	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day2, ClosingPrice: "0"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day3, ClosingPrice: ""})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &day1, ClosingPrice: "12.3456785"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: noCurrencyPK, QuoteDate: nil, ClosingPrice: "5"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	secID := func(pk int64) string { return fmt.Sprintf("sec-%d", pk) }

	assert.Equal(t, map[string]string{
		secID(acmePK):       "Acme Corp|ACME|CAD",
		secID(barePK):       "Bare Fund|NULL|USD",
		secID(noCurrencyPK): "Plain Co|PLN|NULL",
	}, stringMap(t, db, `SELECT id, name || '|' || COALESCE(ticker, 'NULL') || '|' || COALESCE(currency, 'NULL') FROM securities`))
	assert.Equal(t, map[string]string{
		secID(acmePK) + " 2026-03-15": "12.500000",
		secID(acmePK) + " 2026-03-16": "0.000000",
		secID(barePK) + " 2026-03-15": "12.345678",
	}, stringMap(t, db, `SELECT security_id || ' ' || CAST(date AS VARCHAR), CAST(price AS VARCHAR) FROM prices`))
}

func Test_run_sync_records_security_and_price_counts_in_import_runs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day2, ClosingPrice: "13"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &day1, ClosingPrice: "4"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{"counts": "2 3"},
		stringMap(t, db, `SELECT 'counts', concat_ws(' ', securities_rows, prices_rows) FROM import_runs`))
}

func Test_run_sync_imports_investment_transactions_with_named_actions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	expensePK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-50.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: expensePK, Amount: "-50.00"})
	incomePK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: incomePK, Amount: "100.00"})

	invest := func(code int64, row v9fixture.TransactionRow) int64 {
		row.Account = brokeragePK
		row.PostedDate = &day
		row.Type = &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
		return pk
	}
	addSharesPK := invest(2, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0"})
	buyPK := invest(3, v9fixture.TransactionRow{Position: positionPK, Units: "10", Amount: "-1000.50", Commission: "9.99"})
	marginInterestPK := invest(6, v9fixture.TransactionRow{Amount: "-3.25"})
	miscExpensePK := invest(7, v9fixture.TransactionRow{Amount: "-5.00"})
	capitalGainLongPK := invest(8, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "20.00"})
	capitalGainShortPK := invest(9, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "7.50"})
	dividendPK := invest(10, v9fixture.TransactionRow{Amount: "12.00"})
	interestPK := invest(11, v9fixture.TransactionRow{Amount: "1.10"})
	miscIncomePK := invest(12, v9fixture.TransactionRow{Amount: "2.20"})
	reinvestPK := invest(15, v9fixture.TransactionRow{Position: positionPK, Units: "0.5", Amount: "-6.00"})
	removeSharesPK := invest(17, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0"})
	sellPK := invest(19, v9fixture.TransactionRow{Position: positionPK, Units: "-4", Amount: "400.25", Commission: "4.95"})
	splitPK := invest(23, v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "0", Numerator: "1", Denominator: "12"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	itxnID := func(pk int64) string { return fmt.Sprintf("itxn-%d", pk) }
	acme := fmt.Sprintf("sec-%d", acmePK)
	assert.Equal(t, map[string]string{
		itxnID(addSharesPK):        "add_shares|0.000000|0.00|NULL|" + acme + "|NULL|NULL",
		itxnID(buyPK):              "buy|10.000000|-1000.50|9.99|" + acme + "|NULL|NULL",
		itxnID(marginInterestPK):   "margin_interest|NULL|-3.25|NULL|NULL|NULL|NULL",
		itxnID(miscExpensePK):      "misc_expense|NULL|-5.00|NULL|NULL|NULL|NULL",
		itxnID(capitalGainLongPK):  "capital_gain_long|0.000000|20.00|NULL|" + acme + "|NULL|NULL",
		itxnID(capitalGainShortPK): "capital_gain_short|0.000000|7.50|NULL|" + acme + "|NULL|NULL",
		itxnID(dividendPK):         "dividend|NULL|12.00|NULL|NULL|NULL|NULL",
		itxnID(interestPK):         "interest|NULL|1.10|NULL|NULL|NULL|NULL",
		itxnID(miscIncomePK):       "misc_income|NULL|2.20|NULL|NULL|NULL|NULL",
		itxnID(reinvestPK):         "reinvest_dividend|0.500000|-6.00|NULL|" + acme + "|NULL|NULL",
		itxnID(removeSharesPK):     "remove_shares|0.000000|0.00|NULL|" + acme + "|NULL|NULL",
		itxnID(sellPK):             "sell|-4.000000|400.25|4.95|" + acme + "|NULL|NULL",
		itxnID(splitPK):            "split|0.000000|0.00|NULL|" + acme + "|1.000000|12.000000",
	}, stringMap(t, db, `SELECT id, concat_ws('|', action, COALESCE(CAST(shares AS VARCHAR), 'NULL'), CAST(amount AS VARCHAR),
		COALESCE(CAST(commission AS VARCHAR), 'NULL'), COALESCE(security_id, 'NULL'),
		COALESCE(CAST(split_new_shares AS VARCHAR), 'NULL'), COALESCE(CAST(split_old_shares AS VARCHAR), 'NULL'))
		FROM investment_transactions`))

	expenseID := fmt.Sprintf("txn-%d", expensePK)
	incomeID := fmt.Sprintf("txn-%d", incomePK)
	assert.Equal(t, map[string]string{expenseID: "-50.00", incomeID: "100.00"},
		stringMap(t, db, `SELECT id, CAST(amount AS VARCHAR) FROM transactions`))
	assert.Equal(t, map[string]string{expenseID: "50.00"},
		stringMap(t, db, `SELECT transaction_id, CAST(spent AS VARCHAR) FROM v_spending`))
	assert.Equal(t, map[string]string{expenseID: "-50.00", incomeID: "100.00"},
		stringMap(t, db, `SELECT transaction_id, CAST(amount AS VARCHAR) FROM v_cash_flow`))
}
