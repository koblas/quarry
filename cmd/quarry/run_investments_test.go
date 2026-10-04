package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// syncThenReport syncs two groceries rows (plus brokerage rows in the same month when withInvestments), then returns
// spend and cashflow output in text and JSON, and the investment_transactions row count.
func syncThenReport(t *testing.T, withInvestments bool) (map[string]string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	marchTenth := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	for _, cash := range []struct {
		amount string
		day    *time.Time
	}{{"-50.00", &march}, {"-30.00", &marchTenth}} {
		pk := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: cash.amount, PostedDate: cash.day})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: cash.amount, CategoryTag: groceriesPK})
	}
	if withInvestments {
		for _, investment := range []struct {
			code   int64
			amount string
		}{{3, "-400.00"}, {10, "12.00"}} {
			pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: &investment.code, Amount: investment.amount, PostedDate: &marchTenth})
			b.Entry(v9fixture.EntryRow{Parent: pk, Amount: investment.amount, CategoryTag: groceriesPK})
		}
	}
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var syncOut, syncErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncOut, &syncErr), syncErr.String())

	reports := make(map[string]string)
	for _, args := range [][]string{
		{"spend", "--since", "2026-03", "--until", "2026-03"},
		{"spend", "--json", "--since", "2026-03", "--until", "2026-03"},
		{"cashflow", "--since", "2026-03", "--until", "2026-03"},
		{"cashflow", "--json", "--since", "2026-03", "--until", "2026-03"},
	} {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runWith(context.Background(), args, spendEnv(&stdout, &stderr)), stderr.String())
		reports[strings.Join(args, " ")] = stdout.String() + "\n--- stderr ---\n" + stderr.String()
	}

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return reports, stringMap(t, db, `SELECT 'rows', CAST(count(*) AS VARCHAR) FROM investment_transactions`)["rows"]
}

func Test_run_spend_and_cashflow_are_unchanged_by_investment_transactions(t *testing.T) {
	without, withoutRows := syncThenReport(t, false)
	with, withRows := syncThenReport(t, true)

	assert.Equal(t, "0", withoutRows)
	assert.Equal(t, "2", withRows)
	assert.Contains(t, without["spend --since 2026-03 --until 2026-03"], "Groceries")
	assert.Equal(t, without, with)
}

func Test_run_sync_refuses_an_investment_record_quarry_cannot_read(t *testing.T) {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	const brokerage = "an investment transaction on 2026-03-01 in \"Brokerage\""
	cases := []struct {
		name  string
		setup func(b *v9fixture.Builder, brokeragePK int64) string
	}{
		{name: "an action code 14", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(14)), Amount: "1.00", PostedDate: &day})
			return brokerage + " has action code 14, which quarry does not map yet"
		}},
		{name: "1.23456789 shares", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
			positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &day, Position: positionPK, Units: "1.23456789"})
			return brokerage + " has 1.23456789 shares, which has more than 6 decimal places"
		}},
		{name: "a split ratio 1:0", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
			positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(23)), Amount: "0", PostedDate: &day, Position: positionPK, Numerator: "1", Denominator: "0"})
			return `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read (1:0)`
		}},
		{name: "a split ratio with a NULL denominator", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
			positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(23)), Amount: "0", PostedDate: &day, Position: positionPK, Numerator: "1"})
			return `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read (1:none)`
		}},
		{name: "non-zero units and no position", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &day, Units: "2"})
			return brokerage + " has shares but no security"
		}},
		{name: "a security with no name", setup: func(b *v9fixture.Builder, _ int64) string {
			pk := b.Security(v9fixture.SecurityRow{Ticker: "ACME", Currency: "CAD"})
			return fmt.Sprintf("a security (source id %d) has no name", pk)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			quarryDir := filepath.Join(home, "Library", "Application Support", "quarry")
			require.NoError(t, os.MkdirAll(quarryDir, 0o700))
			storePath := filepath.Join(quarryDir, "quarry.duckdb")
			sentinel := []byte("previous store bytes, untouched by an unreadable investment record")
			require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
			b := v9fixture.NewBuilder()
			brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			reason := c.setup(b, brokeragePK)
			bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			id := snapshotID(onlyFileWithSuffix(t, filepath.Join(quarryDir, "snapshots"), ".sqlite"))
			assert.Equal(t,
				"quarry: cannot import snapshot "+id+": "+reason+"; "+abbreviated(t, storePath, home)+
					" was not changed; run quarry sync --from "+id+" once quarry supports it\n",
				stderr.String())
			after, err := os.ReadFile(storePath)
			require.NoError(t, err)
			assert.Equal(t, sentinel, after)
		})
	}
}
