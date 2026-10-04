package duckstore_test

import (
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	secUSD        = "sec-usd"
	secEUR        = "sec-eur"
	secNoCurrency = "sec-none"
	secGhost      = "sec-ghost"
)

// holdingRows is a store whose only investment transactions are txns, with a CAD account and a USD one; its securities are
// Acme (CAD), Globex (USD), Euro Fund (EUR) and Plain Fund (no ticker, no currency), and it has no prices.
func holdingRows(txns ...store.InvestmentTransaction) store.Rows {
	rows := noTransactionRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: acctTwo, SourceID: 2, Name: "Brokerage USD", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true,
	})
	rows.Securities = []store.Security{
		{ID: secAcme, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: secUSD, SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
		{ID: secEUR, SourceID: 3, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
		{ID: secNoCurrency, SourceID: 4, Name: "Plain Fund"},
	}
	rows.Prices = nil
	rows.InvestmentTransactions = txns
	return rows
}

// dayList is dates joined with commas, as heldDaysQuery returns them.
func dayList(days ...time.Time) string {
	texts := make([]string, len(days))
	for i, d := range days {
		texts[i] = d.Format(time.DateOnly)
	}
	return strings.Join(texts, ",")
}

func Test_holdings_view_lists_each_day_of_a_closed_span_from_the_first_to_the_last(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare),
		buy(acctOne, secAcme, 2, marchDay(4), -oneShare)))

	got := queryTexts(t, st, heldDaysQuery)

	assert.Equal(t, [][]string{{dayList(marchDay(1), marchDay(2), marchDay(3))}}, got)
}

func Test_holdings_view_stops_each_span_at_today(t *testing.T) {
	t.Parallel()
	today := localToday()
	daysAgo := func(n int) time.Time { return today.AddDate(0, 0, -n) }
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want string
	}{
		{
			name: "an open span ends today",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, daysAgo(2), oneShare)},
			want: dayList(daysAgo(2), daysAgo(1), today),
		},
		{
			name: "a span closed after today ends today",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, daysAgo(1), oneShare), buy(acctOne, secAcme, 2, today.AddDate(0, 0, 2), -oneShare),
			},
			want: dayList(daysAgo(1), today),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := queryTexts(t, st, heldDaysQuery)

			assert.Equal(t, [][]string{{c.want}}, got)
		})
	}
}

func Test_holdings_view_has_no_rows_for_a_span_that_starts_after_today(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy(acctOne, secAcme, 1, localToday().AddDate(0, 0, 3), oneShare)))

	got := queryTexts(t, st, heldDaysQuery)

	assert.Equal(t, [][]string{{""}}, got)
}

func Test_holdings_view_has_no_rows_between_two_spans_of_one_holding(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(3), -oneShare),
		buy(acctOne, secAcme, 3, marchDay(5), oneShare), buy(acctOne, secAcme, 4, marchDay(7), -oneShare)))

	got := queryTexts(t, st, heldDaysQuery)

	assert.Equal(t, [][]string{{dayList(marchDay(1), marchDay(2), marchDay(5), marchDay(6))}}, got)
}

func Test_holdings_view_has_one_row_a_day_where_two_spans_meet(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(3), oneShare),
		buy(acctOne, secAcme, 3, marchDay(5), -2*oneShare)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), CAST(shares AS VARCHAR) FROM v_holdings ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-01", "1.000000"}, {"2026-03-02", "1.000000"}, {"2026-03-03", "2.000000"}, {"2026-03-04", "2.000000"},
	}, got)
}

func Test_holdings_view_expands_a_span_of_negative_shares(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), -oneShare), buy(acctOne, secAcme, 2, marchDay(3), oneShare)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), CAST(shares AS VARCHAR) FROM v_holdings ORDER BY date")

	assert.Equal(t, [][]string{{"2026-03-01", "-1.000000"}, {"2026-03-02", "-1.000000"}}, got)
}

func Test_holdings_view_lists_a_holding_whose_security_row_is_missing(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secGhost, 1, marchDay(1), oneShare), buy(acctOne, secGhost, 2, marchDay(2), -oneShare)))

	got := queryTexts(t, st, "SELECT security_id, security, ticker, currency, price, value FROM v_holdings")

	assert.Equal(t, [][]string{{secGhost, "NULL", "NULL", "NULL", "NULL", "NULL"}}, got)
}

func Test_holdings_view_lists_accounts_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctTwo, secAcme, 2, marchDay(1), oneShare))
	rows.Accounts[0].NotInReports = true
	rows.Accounts[1].LinkedTracking = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT account_id FROM v_holdings WHERE date = '2026-03-01' ORDER BY account_id")

	assert.Equal(t, [][]string{{acctOne}, {acctTwo}}, got)
}

func Test_holdings_view_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_holdings", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "date", Type: "DATE"},
		{Name: "account_id", Type: "VARCHAR"},
		{Name: "security_id", Type: "VARCHAR"},
		{Name: "security", Type: "VARCHAR"},
		{Name: "ticker", Type: "VARCHAR"},
		{Name: "shares", Type: "DECIMAL(18,6)"},
		{Name: "price", Type: "DECIMAL(18,6)"},
		{Name: "price_date", Type: "DATE"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "value", Type: "DECIMAL(38,2)"},
		{Name: "value_cad", Type: "DECIMAL(38,2)"},
		{Name: "value_usd", Type: "DECIMAL(38,2)"},
		{Name: "usd_cad", Type: "DECIMAL(10,6)"},
	}, got.Columns)
}

func Test_holdings_view_carries_its_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_holdings'")

	assert.Equal(t, [][]string{{"one row per holding per day it is held, through today, so filter by date; " +
		"value is shares times price rounded to the cent, value_cad and value_usd convert it at the rate for date as quarry holdings does; " +
		"cash in investment accounts is not included."}}, got)
}

// heldDaysQuery is the dates of every v_holdings row of Acme, oldest first, joined by commas.
const heldDaysQuery = `SELECT coalesce(string_agg(CAST(date AS VARCHAR), ',' ORDER BY date), '') FROM v_holdings WHERE security_id = 'sec-1'`
