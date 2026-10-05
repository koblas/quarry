package report_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var netWorthDay = time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)

func netWorthOf(t *testing.T, rows []store.NetWorthRow, currency money.Currency) report.NetWorth {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: rows}}))

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Currency: currency})

	require.NoError(t, err)
	return result
}

// cadRow is a CAD row whose balance in CAD is cents; nil leaves it with no conversion.
func cadRow(cents *big.Int) store.NetWorthRow {
	return store.NetWorthRow{Date: netWorthDay, Currency: "CAD", Balance: big.NewInt(1), BalanceCAD: cents}
}

func netWorthTotalValues(result report.NetWorth) []string {
	values := make([]string, len(result.Dates[0].Totals))
	for i, total := range result.Dates[0].Totals {
		values[i] = total.Currency + " " + total.Value.String()
	}
	return values
}

func Test_networth_reads_the_store_once_with_the_day_asked(t *testing.T) {
	var got store.NetWorthParams
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{gotNetWorth: &got, netWorthReads: &reads}))

	_, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.NetWorthParams{Dates: []time.Time{netWorthDay}}, got)
}

func Test_networth_lists_one_date_with_the_rows_read_even_when_there_are_none(t *testing.T) {
	cases := []struct {
		name string
		rows []store.NetWorthRow
	}{
		{name: "no rows", rows: nil},
		{name: "two rows", rows: []store.NetWorthRow{cadRow(big.NewInt(100)), cadRow(big.NewInt(200))}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := netWorthOf(t, c.rows, money.CAD)

			require.Len(t, result.Dates, 1)
			assert.Equal(t, netWorthDay, result.Dates[0].Date)
			assert.Equal(t, c.rows, result.Dates[0].Rows)
			assert.Equal(t, netWorthDay, result.AsOf)
		})
	}
}

func Test_networth_total_is_the_sum_of_the_converted_balances_in_the_reporting_currency(t *testing.T) {
	cases := []struct {
		name     string
		rows     []store.NetWorthRow
		currency money.Currency
		want     []string
	}{
		{name: "two rows add", rows: []store.NetWorthRow{cadRow(big.NewInt(100)), cadRow(big.NewInt(250))}, currency: money.CAD, want: []string{"CAD 350"}},
		{name: "a negative row reduces it", rows: []store.NetWorthRow{cadRow(big.NewInt(100)), cadRow(big.NewInt(-250))}, currency: money.CAD, want: []string{"CAD -150"}},
		{name: "a zero balance is a total of zero", rows: []store.NetWorthRow{cadRow(big.NewInt(0))}, currency: money.CAD, want: []string{"CAD 0"}},
		{name: "a row with no conversion is left out", rows: []store.NetWorthRow{cadRow(big.NewInt(100)), cadRow(nil)}, currency: money.CAD, want: []string{"CAD 100"}},
		{
			name:     "the other currency reads its own column",
			rows:     []store.NetWorthRow{{Date: netWorthDay, Currency: "CAD", BalanceCAD: big.NewInt(125), BalanceUSD: big.NewInt(100)}},
			currency: money.USD, want: []string{"USD 100"},
		},
		{
			name:     "a sum past 64 bits stays exact",
			rows:     []store.NetWorthRow{cadRow(big.NewInt(9_000_000_000_000_000_000)), cadRow(big.NewInt(9_000_000_000_000_000_000))},
			currency: money.CAD, want: []string{"CAD 18000000000000000000"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, netWorthTotalValues(netWorthOf(t, c.rows, c.currency)))
		})
	}
}

func Test_networth_has_no_total_when_no_row_converts(t *testing.T) {
	cases := []struct {
		name     string
		rows     []store.NetWorthRow
		currency money.Currency
	}{
		{name: "no rows", rows: nil, currency: money.CAD},
		{name: "every row unconverted", rows: []store.NetWorthRow{cadRow(nil), cadRow(nil)}, currency: money.CAD},
		{name: "only the other currency converts", rows: []store.NetWorthRow{cadRow(big.NewInt(100))}, currency: money.USD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, netWorthOf(t, c.rows, c.currency).Dates[0].Totals)
		})
	}
}

func Test_networth_native_totals_each_currency_on_its_own(t *testing.T) {
	rows := []store.NetWorthRow{
		{Date: netWorthDay, Type: "brokerage", Currency: "USD", Balance: big.NewInt(300), BalanceCAD: big.NewInt(408)},
		{Date: netWorthDay, Type: "chequing", Currency: "CAD", Balance: big.NewInt(100), BalanceCAD: big.NewInt(100)},
		{Date: netWorthDay, Type: "chequing", Currency: "USD", Balance: big.NewInt(-50), BalanceCAD: big.NewInt(-68)},
		{Date: netWorthDay, Type: "savings", Currency: "CAD", Balance: big.NewInt(25), BalanceCAD: big.NewInt(25)},
	}

	result := netWorthOf(t, rows, money.Native)

	assert.Equal(t, []string{"CAD 125", "USD 250"}, netWorthTotalValues(result))
}

func Test_networth_native_has_no_total_without_rows(t *testing.T) {
	assert.Empty(t, netWorthOf(t, nil, money.Native).Dates[0].Totals)
}

func Test_networth_converted_is_nil_in_a_native_listing(t *testing.T) {
	listing := report.NetWorth{Currency: money.Native}

	got := listing.Converted(store.NetWorthRow{BalanceCAD: big.NewInt(1), BalanceUSD: big.NewInt(2)})

	assert.Nil(t, got)
}

func Test_networth_refuses_when_the_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.NetWorth(t.Context(), report.NetWorthRequest{})

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_networth_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.NetWorth(ctx, report.NetWorthRequest{})

	assert.EqualError(t, err, "networth interrupted")
}

func Test_networth_returns_any_other_read_failure_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.NetWorth(t.Context(), report.NetWorthRequest{})

	assert.Equal(t, errDiskRead, err)
}

var netWorthHistory = store.Window{Since: day(2026, time.January, 1), Until: netWorthDay}

func Test_networth_history_reads_the_store_once_with_every_month_end(t *testing.T) {
	var got store.NetWorthParams
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{gotNetWorth: &got, netWorthReads: &reads}))

	_, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &netWorthHistory})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.NetWorthParams{Dates: []time.Time{day(2026, time.January, 31), day(2026, time.February, 28), netWorthDay}}, got)
}

func Test_networth_history_files_each_row_under_its_day_and_lists_a_day_with_none(t *testing.T) {
	january := cadRow(big.NewInt(100))
	january.Date = day(2026, time.January, 31)
	today := cadRow(big.NewInt(300))
	today.Date = time.Date(2026, time.March, 12, 0, 0, 0, 0, time.FixedZone("elsewhere", 0))
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: []store.NetWorthRow{january, today}}}))

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &netWorthHistory, Currency: money.CAD})

	require.NoError(t, err)
	require.Len(t, result.Dates, 3)
	assert.Equal(t, []time.Time{day(2026, time.January, 31), day(2026, time.February, 28), netWorthDay},
		[]time.Time{result.Dates[0].Date, result.Dates[1].Date, result.Dates[2].Date})
	assert.Equal(t, [][]store.NetWorthRow{{january}, nil, {today}}, [][]store.NetWorthRow{result.Dates[0].Rows, result.Dates[1].Rows, result.Dates[2].Rows})
	assert.Equal(t, &netWorthHistory, result.Window)
}

func Test_networth_history_totals_each_day_on_its_own(t *testing.T) {
	january := cadRow(big.NewInt(100))
	january.Date = day(2026, time.January, 31)
	today := cadRow(big.NewInt(300))
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: []store.NetWorthRow{january, today}}}))

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &netWorthHistory, Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, [][]report.NetWorthTotal{
		{{Currency: "CAD", Value: big.NewInt(100)}}, nil, {{Currency: "CAD", Value: big.NewInt(300)}},
	}, [][]report.NetWorthTotal{result.Dates[0].Totals, result.Dates[1].Totals, result.Dates[2].Totals})
}

func Test_networth_history_of_a_window_with_no_month_end_reads_no_dates_and_lists_none(t *testing.T) {
	var got store.NetWorthParams
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{gotNetWorth: &got, netWorthReads: &reads}))
	future := store.Window{Since: day(2027, time.January, 1), Until: netWorthDay}

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &future})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Empty(t, got.Dates)
	assert.Empty(t, result.Dates)
}

func Test_networth_history_refuses_when_the_read_fails(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &netWorthHistory})

	assert.Equal(t, errDiskRead, err)
}

func typedRow(accountType, currency string, balance int64, cad *big.Int) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: currency, Balance: big.NewInt(balance), BalanceCAD: cad}
}

func Test_networth_types_are_those_with_a_balance_on_some_day_alphabetically(t *testing.T) {
	listing := report.NetWorth{Dates: []report.NetWorthDate{
		{Rows: []store.NetWorthRow{typedRow("savings", "CAD", 5, nil), typedRow("brokerage", "CAD", 0, nil), typedRow("chequing", "CAD", 0, nil)}},
		{Rows: []store.NetWorthRow{typedRow("savings", "USD", 7, nil), typedRow("chequing", "CAD", -2, nil), typedRow("brokerage", "CAD", 0, nil)}},
	}}

	assert.Equal(t, []string{"chequing", "savings"}, listing.Types())
}

func Test_networth_types_are_none_when_every_balance_is_zero(t *testing.T) {
	listing := report.NetWorth{Dates: []report.NetWorthDate{{Rows: []store.NetWorthRow{typedRow("chequing", "CAD", 0, nil)}}}}

	assert.Empty(t, listing.Types())
}

func Test_networth_type_converted_sums_the_type_over_currencies_in_the_reporting_currency(t *testing.T) {
	listing := report.NetWorth{Currency: money.CAD}
	date := report.NetWorthDate{Rows: []store.NetWorthRow{
		typedRow("chequing", "CAD", 1, big.NewInt(100)), typedRow("chequing", "USD", 1, big.NewInt(150)),
		typedRow("savings", "CAD", 1, big.NewInt(9)),
	}}

	assert.Equal(t, big.NewInt(250), listing.TypeConverted(date, "chequing"))
}

func Test_networth_type_converted_leaves_out_a_row_no_rate_converts(t *testing.T) {
	listing := report.NetWorth{Currency: money.CAD}
	date := report.NetWorthDate{Rows: []store.NetWorthRow{typedRow("chequing", "CAD", 1, big.NewInt(100)), typedRow("chequing", "USD", 1, nil)}}

	assert.Equal(t, big.NewInt(100), listing.TypeConverted(date, "chequing"))
}

func Test_networth_type_converted_is_nil_when_nothing_of_the_type_converts(t *testing.T) {
	cases := []struct {
		name    string
		listing report.NetWorth
		rows    []store.NetWorthRow
	}{
		{name: "the type has no row", listing: report.NetWorth{Currency: money.CAD}, rows: []store.NetWorthRow{typedRow("savings", "CAD", 1, big.NewInt(9))}},
		{name: "no row of the type converts", listing: report.NetWorth{Currency: money.CAD}, rows: []store.NetWorthRow{typedRow("chequing", "USD", 1, nil)}},
		{name: "a native listing converts none", listing: report.NetWorth{Currency: money.Native}, rows: []store.NetWorthRow{typedRow("chequing", "CAD", 1, big.NewInt(9))}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Nil(t, c.listing.TypeConverted(report.NetWorthDate{Rows: c.rows}, "chequing"))
		})
	}
}

func Test_networth_type_balance_is_the_balance_of_that_type_and_currency(t *testing.T) {
	date := report.NetWorthDate{Rows: []store.NetWorthRow{
		typedRow("chequing", "CAD", 100, nil), typedRow("chequing", "USD", 250, nil), typedRow("savings", "USD", 9, nil),
	}}

	assert.Equal(t, big.NewInt(250), date.TypeBalance("chequing", "USD"))
}

func Test_networth_type_balance_is_nil_without_a_row_for_the_type_and_currency(t *testing.T) {
	date := report.NetWorthDate{Rows: []store.NetWorthRow{typedRow("chequing", "CAD", 100, nil), typedRow("savings", "USD", 9, nil)}}

	assert.Nil(t, date.TypeBalance("chequing", "USD"))
	assert.Nil(t, date.TypeBalance("brokerage", "CAD"))
}
