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
