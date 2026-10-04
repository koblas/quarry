package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "0.541667"})

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
		itxnID(buyPK):              "buy|10.000000|-1000.50|9.9900|" + acme + "|NULL|NULL",
		itxnID(marginInterestPK):   "margin_interest|NULL|-3.25|NULL|NULL|NULL|NULL",
		itxnID(miscExpensePK):      "misc_expense|NULL|-5.00|NULL|NULL|NULL|NULL",
		itxnID(capitalGainLongPK):  "capital_gain_long|0.000000|20.00|NULL|" + acme + "|NULL|NULL",
		itxnID(capitalGainShortPK): "capital_gain_short|0.000000|7.50|NULL|" + acme + "|NULL|NULL",
		itxnID(dividendPK):         "dividend|NULL|12.00|NULL|NULL|NULL|NULL",
		itxnID(interestPK):         "interest|NULL|1.10|NULL|NULL|NULL|NULL",
		itxnID(miscIncomePK):       "misc_income|NULL|2.20|NULL|NULL|NULL|NULL",
		itxnID(reinvestPK):         "reinvest_dividend|0.500000|-6.00|NULL|" + acme + "|NULL|NULL",
		itxnID(removeSharesPK):     "remove_shares|0.000000|0.00|NULL|" + acme + "|NULL|NULL",
		itxnID(sellPK):             "sell|-4.000000|400.25|4.9500|" + acme + "|NULL|NULL",
		itxnID(splitPK):            "split|0.000000|0.00|NULL|" + acme + "|1.000000|12.000000",
	}, stringMap(t, db, `SELECT id, concat_ws('|', action, COALESCE(CAST(shares AS VARCHAR), 'NULL'), CAST(amount AS VARCHAR),
		COALESCE(CAST(commission AS VARCHAR), 'NULL'), COALESCE(security_id, 'NULL'),
		COALESCE(CAST(split_new_shares AS VARCHAR), 'NULL'), COALESCE(CAST(split_old_shares AS VARCHAR), 'NULL'))
		FROM investment_transactions`))

	cashID := func(pk int64) string { return fmt.Sprintf("txn-%d", pk) }
	assert.Equal(t, map[string]string{
		cashID(expensePK):          "-50.00",
		cashID(incomePK):           "100.00",
		cashID(buyPK):              "-1000.50",
		cashID(marginInterestPK):   "-3.25",
		cashID(miscExpensePK):      "-5.00",
		cashID(capitalGainLongPK):  "20.00",
		cashID(capitalGainShortPK): "7.50",
		cashID(dividendPK):         "12.00",
		cashID(interestPK):         "1.10",
		cashID(miscIncomePK):       "2.20",
		cashID(reinvestPK):         "-6.00",
		cashID(sellPK):             "400.25",
	}, stringMap(t, db, `SELECT id, CAST(amount AS VARCHAR) FROM transactions`))
}

// syncThenReport syncs two groceries rows (plus brokerage rows that move no cash in the same month when
// withInvestments), then returns spend and cashflow output in text and JSON, and the investment_transactions row count.
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
		}{{2, "0"}, {17, "0"}} {
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

func Test_run_spend_and_cashflow_are_unchanged_by_investment_transactions_that_move_no_cash(t *testing.T) {
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
		{name: "a commission of 1.23456", setup: func(b *v9fixture.Builder, brokeragePK int64) string {
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &day, Commission: "1.23456"})
			return brokerage + " has a commission of 1.23456, which has more than 4 decimal places"
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

// The lots equal each holding's derived share count, so the share-count gate passes.
func holdingsBundle(t *testing.T, home string) v9fixture.Bundle {
	t.Helper()
	day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	rrspPK := b.Account(v9fixture.AccountRow{Name: "Old RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Closed: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "CAD"})
	acmePosition := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	barePosition := b.Position(v9fixture.PositionRow{Account: rrspPK, Security: barePK})

	cashPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-50.00", PostedDate: &day1})
	b.Entry(v9fixture.EntryRow{Parent: cashPK, Amount: "-50.00"})
	invest := func(code int64, row v9fixture.TransactionRow) {
		row.PostedDate, row.Type = &day1, &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	}
	invest(3, v9fixture.TransactionRow{Account: brokeragePK, Position: acmePosition, Units: "10", Amount: "-1000.00"})
	invest(19, v9fixture.TransactionRow{Account: brokeragePK, Position: acmePosition, Units: "-4", Amount: "400.00"})
	invest(3, v9fixture.TransactionRow{Account: rrspPK, Position: barePosition, Units: "5", Amount: "-50.00"})
	b.Lot(v9fixture.LotRow{Position: acmePosition, LatestUnits: "4"})
	b.Lot(v9fixture.LotRow{Position: acmePosition, LatestUnits: "2"})
	b.Lot(v9fixture.LotRow{Position: barePosition, LatestUnits: "5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day1, ClosingPrice: "100"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day2, ClosingPrice: "101"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &day1, ClosingPrice: "10"})

	return b.WriteBundle(t, filepath.Join(home, "Documents"))
}

func Test_run_sync_reports_holdings_that_match_quickens_share_counts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := holdingsBundle(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(),
		"Rows      4 transactions, 4 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 3 investment transactions, 2 securities, 3 prices\n")
	assert.Contains(t, stdout.String(), "Shares    2 holdings match Quicken's share counts\n")

	var jsonOut, jsonErr bytes.Buffer
	exitCode = run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &jsonOut, &jsonErr)

	require.Equal(t, 0, exitCode, jsonErr.String())
	var doc struct {
		Store struct {
			Rows   map[string]int  `json:"rows"`
			Shares json.RawMessage `json:"shares"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.JSONEq(t, `{"checked":2,"mismatched":[]}`, string(doc.Store.Shares))
	assert.Equal(t, 3, doc.Store.Rows["investment_transactions"])
	assert.Equal(t, 2, doc.Store.Rows["securities"])
	assert.Equal(t, 3, doc.Store.Rows["prices"])
}

// oneHoldingBundle is a reconciled chequing account that matches its statement
// plus a 10-share holding whose only lot reads lotUnits.
func oneHoldingBundle(t *testing.T, home, lotUnits string) v9fixture.Bundle {
	t.Helper()
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	cashPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: cashPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	buyPK := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: brokeragePK, Position: positionPK, Type: new(int64(3)), Units: "10", Amount: "-1000.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyPK, Amount: "-1000.00"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: lotUnits})
	return b.WriteBundle(t, filepath.Join(home, "Documents"))
}

// Cash checks pass here, so a kept store can only come from the share-count gate.
func Test_run_sync_leaves_the_previous_store_byte_identical_when_share_counts_differ(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storePath := storePathUnder(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o700))
	sentinel := []byte("previous store bytes, untouched by a failing sync")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	bundle := oneHoldingBundle(t, home, "9")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stdout.String(), "NOT REBUILT")
	got, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, got)
}

func Test_run_sync_replaces_the_store_when_the_lot_matches_the_derived_share_count(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storePath := storePathUnder(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o700))
	sentinel := []byte("previous store bytes, replaced by a passing sync")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	bundle := oneHoldingBundle(t, home, "10")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	got, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.NotEqual(t, sentinel, got)
}

func Test_run_sync_applies_a_stock_split_in_date_order(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	buyDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	splitDay := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	sellDay := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	invest := func(code int64, day time.Time, row v9fixture.TransactionRow) {
		row.Account, row.Position, row.PostedDate, row.Type = brokeragePK, positionPK, &day, &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	}
	invest(19, sellDay, v9fixture.TransactionRow{Units: "-10", Amount: "100.00"})
	invest(3, buyDay, v9fixture.TransactionRow{Units: "120", Amount: "-1200.00"})
	invest(23, splitDay, v9fixture.TransactionRow{Units: "0", Amount: "0", Numerator: "1", Denominator: "12"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "0"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Shares    1 holding matches Quicken's share count\n")
}

func Test_run_sync_reports_a_file_with_no_investment_data(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(),
		"Rows      0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices\n")
	assert.Contains(t, stdout.String(), "Shares    no holdings to check\n")
}
