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
	investmentCodeAddShares = 2
	investmentCodeBuy       = 3
	investmentCodeDividend  = 10
	investmentCodeReinvest  = 15
	investmentCodeRemove    = 17
	investmentCodeSplit     = 23

	categoryKindIncome = 2
	categoryKindSystem = 0
)

func Test_run_sync_gives_each_investment_transaction_that_moves_cash_a_row_in_transactions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

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
		stringMap(t, db, `SELECT 'cash', CAST(SUM(amount) AS VARCHAR) FROM transactions WHERE account_id = 'acct-`+fmt.Sprint(brokeragePK)+`'`))
	assert.Equal(t, []string{"8"}, storeTextRows(t, home, "SELECT CAST(format_version AS VARCHAR) FROM store_info"))
}

func Test_run_sync_gives_a_reinvested_dividend_no_row_and_no_income(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{fmt.Sprintf("txn-%d", dividendPK): "12.00"},
		stringMap(t, db, `SELECT id, CAST(amount AS VARCHAR) FROM transactions`))
	assert.Equal(t, map[string]string{"Dividends": "12.00"},
		stringMap(t, db, `SELECT category, CAST(SUM(amount) AS VARCHAR) FROM v_cash_flow WHERE flow = 'income' GROUP BY category`))
}
