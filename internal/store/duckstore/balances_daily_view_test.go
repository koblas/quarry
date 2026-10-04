package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const (
	acctChequing   = "acct-chq"
	acctRetirement = "acct-ret"
	acctEUR        = "acct-eur"
	secControl     = "sec-control"
)

// balanceRows is holdingRows with brokerage accounts acct-1 (CAD) and acct-2 (USD), a CAD chequing account, a CAD
// retirement account and a EUR brokerage account; Control Corp (CAD) joins its securities.
func balanceRows(txns ...store.InvestmentTransaction) store.Rows {
	rows := holdingRows(txns...)
	rows.Accounts[0].Type = store.AccountTypeBrokerage
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctChequing, SourceID: 3, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
		store.Account{ID: acctRetirement, SourceID: 4, Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Active: true},
		store.Account{ID: acctEUR, SourceID: 5, Name: "Euro Brokerage", Type: store.AccountTypeBrokerage, Currency: "EUR", Active: true})
	rows.Securities = append(rows.Securities,
		store.Security{ID: secControl, SourceID: 5, Name: "Control Corp", Ticker: new("CTRL"), Currency: new("CAD")})
	return rows
}

// cashRows is a store whose only account is a CAD chequing account with txns.
func cashRows(txns ...store.Transaction) store.Rows {
	rows := noTransactionRows()
	rows.Transactions = txns
	return rows
}

// dayQuery is the first day, last day and row count of v_balances_daily for account.
func dayQuery(account string) string {
	return fmt.Sprintf("SELECT CAST(min(date) AS VARCHAR), CAST(max(date) AS VARCHAR), count(*) = count(DISTINCT date) FROM v_balances_daily WHERE account_id = '%s'", account)
}

func Test_balances_daily_runs_from_the_first_transaction_through_today(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(3), 100)))

	got := queryTexts(t, st, dayQuery(acctOne))

	assert.Equal(t, [][]string{{"2026-03-03", localToday().Format(time.DateOnly), "true"}}, got)
}

func Test_balances_daily_starts_on_the_earlier_of_the_first_transaction_and_the_first_holding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		txnDay      int
		holdingDay  int
		wantFirstOn string
	}{
		{name: "the transaction comes first", txnDay: 2, holdingDay: 5, wantFirstOn: "2026-03-02"},
		{name: "the holding comes first", txnDay: 5, holdingDay: 2, wantFirstOn: "2026-03-02"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(c.holdingDay), oneShare))
			rows.Transactions = []store.Transaction{transaction("t1", acctOne, marchDay(c.txnDay), 100)}
			st := newStoreWith(t, rows)

			got := queryTexts(t, st, dayQuery(acctOne))

			assert.Equal(t, [][]string{{c.wantFirstOn, localToday().Format(time.DateOnly), "true"}}, got)
		})
	}
}

func Test_balances_daily_starts_on_the_first_holding_when_the_account_has_no_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows(buy(acctOne, secAcme, 1, marchDay(4), oneShare)))

	got := queryTexts(t, st, dayQuery(acctOne))

	assert.Equal(t, [][]string{{"2026-03-04", localToday().Format(time.DateOnly), "true"}}, got)
}

func Test_balances_daily_has_no_rows_for_an_account_with_no_transaction_and_no_holding(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows(buy(acctOne, secAcme, 1, marchDay(4), oneShare)))

	got := queryTexts(t, st, "SELECT count(*) FROM v_balances_daily WHERE account_id = '"+acctTwo+"'")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_balances_daily_has_no_rows_for_an_account_whose_only_transaction_is_future_dated(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, localToday().AddDate(0, 0, 3), 100)))

	got := queryTexts(t, st, "SELECT count(*) FROM v_balances_daily")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_balances_daily_lists_closed_left_out_and_linked_tracking_accounts(t *testing.T) {
	t.Parallel()
	rows := cashRows(
		transaction("t1", acctOne, marchDay(1), 100), transaction("t2", acctTwo, marchDay(1), 100),
		transaction("t3", acctChequing, marchDay(1), 100))
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctTwo, SourceID: 2, Name: "Not In Reports", Type: "chequing", Currency: "CAD", Active: true, NotInReports: true},
		store.Account{ID: acctChequing, SourceID: 3, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true})
	rows.Accounts[0].Closed = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT DISTINCT account_id FROM v_balances_daily ORDER BY account_id")

	assert.Equal(t, [][]string{{acctOne}, {acctTwo}, {acctChequing}}, got)
}

func Test_balances_daily_cash_is_the_running_sum_of_transactions_on_or_before_each_day(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctOne, marchDay(3), 5_000),
		transaction("t3", acctOne, marchDay(3), -250)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), cash FROM v_balances_daily WHERE date <= '2026-03-04' ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-01", "100.00"}, {"2026-03-02", "100.00"}, {"2026-03-03", "147.50"}, {"2026-03-04", "147.50"},
	}, got)
}

func Test_balances_daily_cash_includes_a_transaction_excluded_from_reports(t *testing.T) {
	t.Parallel()
	excluded := transaction("t2", acctOne, marchDay(1), 5_000)
	excluded.ExcludedFromReports = true
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000), excluded))

	got := queryTexts(t, st, "SELECT cash FROM v_balances_daily WHERE date = '2026-03-01'")

	assert.Equal(t, [][]string{{"150.00"}}, got)
}

func Test_balances_daily_cash_leaves_out_a_transaction_dated_after_today(t *testing.T) {
	t.Parallel()
	today := localToday()
	st := newStoreWith(t, cashRows(
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctOne, today.AddDate(0, 0, 2), 50_000),
		transaction("t3", acctOne, today, 700)))

	got := queryTexts(t, st, "SELECT cash FROM v_balances_daily WHERE date = current_date")

	assert.Equal(t, [][]string{{"107.00"}}, got)
}

func Test_balances_daily_has_no_holdings_figures_outside_brokerage_and_retirement_accounts(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000)))

	got := queryTexts(t, st, "SELECT holdings_value, holdings_unvalued FROM v_balances_daily WHERE date = '2026-03-01'")

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

// holdingsQuery is account's holdings_value and holdings_unvalued on date.
func holdingsQuery(account, date string) string {
	return fmt.Sprintf("SELECT holdings_value, holdings_unvalued FROM v_balances_daily WHERE account_id = '%s' AND date = '%s'", account, date)
}

func Test_balances_daily_counts_each_holding_it_cannot_value(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		account string
		txns    []store.InvestmentTransaction
		prices  []store.Price
	}{
		{
			name: "a holding with no price", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, marchDay(1), tenUnits)},
		},
		{
			name: "a holding with no currency", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, secNoCurrency, 2, marchDay(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, marchDay(1), tenUnits), quote(secNoCurrency, 2, marchDay(1), tenUnits)},
		},
		{
			name: "a EUR holding in a CAD account", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, secEUR, 2, marchDay(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, marchDay(1), tenUnits), quote(secEUR, 2, marchDay(1), tenUnits)},
		},
		{
			name: "a USD holding in a CAD account before the first rate", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, secUSD, 2, marchDay(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, marchDay(1), tenUnits), quote(secUSD, 2, marchDay(1), tenUnits)},
		},
		{
			name: "a CAD holding in a USD account before the first rate", account: acctTwo,
			txns:   []store.InvestmentTransaction{buy(acctTwo, secUSD, 1, marchDay(1), oneShare), buy(acctTwo, secControl, 2, marchDay(1), oneShare)},
			prices: []store.Price{quote(secUSD, 1, marchDay(1), tenUnits), quote(secControl, 2, marchDay(1), tenUnits)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(c.txns...)
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, holdingsQuery(c.account, "2026-03-02"))

			assert.Equal(t, [][]string{{"10.00", "1"}}, got)
		})
	}
}

func Test_balances_daily_values_holdings_in_the_accounts_currency(t *testing.T) {
	t.Parallel()
	const oneAndATenth = 10_010_000
	cases := []struct {
		name         string
		account      string
		txns         []store.InvestmentTransaction
		prices       []store.Price
		wantValue    string
		wantUnvalued string
		onMarch      int
	}{
		{
			name: "an own-currency holding as is", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, marchDay(1), 3*oneShare)},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)},
			wantValue: "30.00", wantUnvalued: "0",
		},
		{
			name: "a USD holding in a CAD account converted at the rate and rounded", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secUSD, 1, marchDay(1), oneShare)},
			prices:    []store.Price{quote(secUSD, 1, marchDay(1), oneAndATenth)},
			wantValue: "12.51", wantUnvalued: "0",
		},
		{
			name: "a CAD holding in a USD account converted at the rate and rounded", account: acctTwo, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctTwo, secAcme, 1, marchDay(1), oneShare)},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), oneAndATenth)},
			wantValue: "8.01", wantUnvalued: "0",
		},
		{
			name: "a retirement account valued like a brokerage account", account: acctRetirement, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctRetirement, secAcme, 1, marchDay(1), 2*oneShare)},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)},
			wantValue: "20.00", wantUnvalued: "0",
		},
		{
			name: "a EUR holding in a EUR account as is", account: acctEUR, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctEUR, secEUR, 1, marchDay(1), oneShare)},
			prices:    []store.Price{quote(secEUR, 1, marchDay(1), tenUnits)},
			wantValue: "10.00", wantUnvalued: "0",
		},
		{
			name: "a CAD holding in a EUR account left out", account: acctEUR, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctEUR, secAcme, 1, marchDay(1), oneShare)},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)},
			wantValue: "0.00", wantUnvalued: "1",
		},
		{
			name: "nothing held on a day between spans", account: acctOne, onMarch: 5,
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(3), -oneShare),
				buy(acctOne, secControl, 3, marchDay(7), oneShare),
			},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), tenUnits), quote(secControl, 2, marchDay(7), tenUnits)},
			wantValue: "0.00", wantUnvalued: "0",
		},
		{
			name: "every holding unvalued", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, marchDay(1), oneShare)},
			prices:    nil,
			wantValue: "0.00", wantUnvalued: "1",
		},
		{
			name: "the largest holding without overflow", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, marchDay(1), maxDecimal18x6)},
			prices:    []store.Price{quote(secAcme, 1, marchDay(1), maxDecimal18x6)},
			wantValue: maxHoldingValue, wantUnvalued: "0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(c.txns...)
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

			got := queryTexts(t, st, holdingsQuery(c.account, fmt.Sprintf("2026-03-%02d", c.onMarch)))

			assert.Equal(t, [][]string{{c.wantValue, c.wantUnvalued}}, got)
		})
	}
}
