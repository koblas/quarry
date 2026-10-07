package report_test

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func holdingsOf(t *testing.T, rows []store.Holding, currency money.Currency) report.Holdings {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{holdings: store.Holdings{Holdings: rows}}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Currency: currency})

	require.NoError(t, err)
	return result
}

// cadHolding is a holding whose value in CAD is cents; nil leaves it with no conversion.
func cadHolding(cents *big.Int) store.Holding { return store.Holding{ValueCAD: cents} }

func totalValues(result report.Holdings) []string {
	values := make([]string, len(result.Totals))
	for i, total := range result.Totals {
		values[i] = total.Currency + " " + total.Value.String()
	}
	return values
}

// ownHolding is a holding priced in currency code, worth cents in it; a nil code is a security with no currency.
func ownHolding(code *string, cents *big.Int) store.Holding {
	return store.Holding{Currency: code, Value: cents}
}

func Test_holdings_reads_the_store_once_for_the_day_asked_and_returns_it_with_the_currency(t *testing.T) {
	var got store.HoldingsParams
	var reads int
	asOf := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
	srv := report.NewServer(report.WithStore(fakeStore{gotHoldings: &got, holdingsReads: &reads}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{AsOf: asOf, Currency: money.USD})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.HoldingsParams{AsOf: asOf}, got)
	assert.Equal(t, report.Holdings{AsOf: asOf, Currency: money.USD}, result)
}

func Test_holdings_keeps_the_stores_rows_in_the_stores_order(t *testing.T) {
	rows := []store.Holding{{Account: "Zeta"}, {Account: "Alpha"}}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, rows, result.Rows)
}

func Test_holdings_converts_each_value_to_the_asked_currency(t *testing.T) {
	row := store.Holding{Value: big.NewInt(100), ValueCAD: big.NewInt(136), ValueUSD: big.NewInt(100)}
	cases := []struct {
		name     string
		currency money.Currency
		want     *big.Int
	}{
		{name: "CAD reads the CAD value", currency: money.CAD, want: big.NewInt(136)},
		{name: "USD reads the USD value", currency: money.USD, want: big.NewInt(100)},
		{name: "native converts nothing", currency: money.Native, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := report.Holdings{Currency: c.currency}

			assert.Equal(t, c.want, result.Converted(row))
		})
	}
}

func Test_Convertible_is_true_only_for_a_security_in_cad_or_usd(t *testing.T) {
	cases := []struct {
		name     string
		currency *string
		want     bool
	}{
		{name: "CAD", currency: new("CAD"), want: true},
		{name: "USD", currency: new("USD"), want: true},
		{name: "EUR", currency: new("EUR"), want: false},
		{name: "no currency", currency: nil, want: false},
		{name: "lower case is not CAD", currency: new("cad"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.Convertible(store.Holding{Currency: c.currency}))
		})
	}
}

func Test_holdings_has_no_total_when_no_row_converts(t *testing.T) {
	cases := []struct {
		name     string
		rows     []store.Holding
		currency money.Currency
	}{
		{name: "no rows", rows: nil, currency: money.CAD},
		{name: "every row unconverted", rows: []store.Holding{cadHolding(nil), cadHolding(nil)}, currency: money.CAD},
		{name: "only the other currency converts", rows: []store.Holding{cadHolding(big.NewInt(100))}, currency: money.USD},
		{name: "a native listing", rows: []store.Holding{{Value: big.NewInt(100), ValueCAD: big.NewInt(100)}}, currency: money.Native},
		{name: "an unpriced holding has no unconverted entry", rows: []store.Holding{{Currency: new("USD")}}, currency: money.CAD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, holdingsOf(t, c.rows, c.currency).Totals)
		})
	}
}

func Test_holdings_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Holdings(ctx, report.HoldingsRequest{})

	assert.EqualError(t, err, "holdings interrupted")
}

func Test_holdings_returns_any_other_read_failure_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Holdings(t.Context(), report.HoldingsRequest{})

	assert.Equal(t, errDiskRead, err)
}

func holdingsAccounts(t *testing.T, list store.AccountList, names ...string) (report.Holdings, store.HoldingsParams, int, error) {
	t.Helper()
	var got store.HoldingsParams
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, gotHoldings: &got, holdingsReads: &reads}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Accounts: names})

	return result, got, reads, err
}

func Test_holdings_names_the_accounts_given_by_name_id_and_name_ignoring_case_in_the_order_given(t *testing.T) {
	list := accountsOf(chequing, savings, oldCard)

	result, got, reads, err := holdingsAccounts(t, list, "Old Card", "acct-100", "SAVINGS")

	require.NoError(t, err)
	assert.Equal(t, []store.Account{oldCard, chequing, savings}, result.Accounts)
	assert.Equal(t, []string{"acct-300", "acct-100", "acct-200"}, got.AccountIDs)
	assert.Equal(t, 1, reads)
}

func Test_holdings_names_an_account_given_by_name_and_by_id_once(t *testing.T) {
	list := accountsOf(chequing, savings)

	result, got, _, err := holdingsAccounts(t, list, "chequing", "acct-100", "Chequing")

	require.NoError(t, err)
	assert.Equal(t, []store.Account{chequing}, result.Accounts)
	assert.Equal(t, []string{"acct-100"}, got.AccountIDs)
}

func Test_holdings_refuses_an_unknown_or_empty_account_without_reading_holdings(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{name: "an unknown name", arg: "Chequeing"},
		{name: "an empty argument", arg: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, reads, err := holdingsAccounts(t, accountsOf(chequing), "Chequing", c.arg)

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
			assert.Equal(t, c.arg, refusal.Arg)
			assert.Zero(t, reads)
		})
	}
}

func Test_holdings_refuses_an_ambiguous_account_name(t *testing.T) {
	list := accountsOf(store.Account{ID: "acct-977", Name: "Visa"}, store.Account{ID: "acct-812", Name: "visa"})

	_, _, reads, err := holdingsAccounts(t, list, "VISA")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, report.RefusalAmbiguousAccount, refusal.Kind)
	assert.Equal(t, []string{"acct-812", "acct-977"}, refusal.IDs)
	assert.Zero(t, reads)
}

func Test_holdings_does_not_read_accounts_when_none_is_named(t *testing.T) {
	var accountsReads int
	var got store.HoldingsParams
	srv := report.NewServer(report.WithStore(fakeStore{accountsReads: &accountsReads, gotHoldings: &got}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{})

	require.NoError(t, err)
	assert.Zero(t, accountsReads)
	assert.Nil(t, got.AccountIDs)
	assert.Nil(t, result.Accounts)
}

// pricedHolding is a priced holding in currency code worth cents in it, with no conversion; a nil code is no currency.
func pricedHolding(code *string, cents int64) store.Holding {
	return store.Holding{Currency: code, Price: new(int64(1_000_000)), Value: big.NewInt(cents)}
}

func Test_holdings_carries_the_first_rate_date_the_store_read(t *testing.T) {
	first := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	srv := report.NewServer(report.WithStore(fakeStore{holdings: store.Holdings{FirstRate: first}}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, first, result.FirstRate)
}

func Test_holdings_carries_the_transaction_span_the_store_read_from_one_read(t *testing.T) {
	first, last := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{
		holdings: store.Holdings{FirstTransaction: first, LastTransaction: last}, holdingsReads: &reads,
	}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, []time.Time{first, last}, []time.Time{result.FirstTransaction, result.LastTransaction})
	assert.Equal(t, 1, reads)
}

// withValueCAD is h with a CAD value of cents.
func withValueCAD(h store.Holding, cents int64) store.Holding {
	h.ValueCAD = big.NewInt(cents)
	return h
}

// withValueUSD is h with a USD value of cents.
func withValueUSD(h store.Holding, cents int64) store.Holding {
	h.ValueUSD = big.NewInt(cents)
	return h
}

func Test_holdings_totals_by_currency(t *testing.T) {
	cases := []struct {
		name     string
		rows     []store.Holding
		currency money.Currency
		want     []string
	}{
		{
			name: "native totals each stored currency cad then usd then alphabetically", currency: money.Native,
			rows: []store.Holding{
				ownHolding(new("GBP"), big.NewInt(1)), ownHolding(new("EUR"), big.NewInt(2)), ownHolding(new("USD"), big.NewInt(3)),
				ownHolding(new("AUD"), big.NewInt(4)), ownHolding(new("CAD"), big.NewInt(5)),
			},
			want: []string{"CAD 5", "USD 3", "AUD 4", "EUR 2", "GBP 1"},
		},
		{
			name: "native sums the values of one currency", currency: money.Native,
			rows: []store.Holding{ownHolding(new("CAD"), big.NewInt(100)), ownHolding(new("USD"), big.NewInt(7)), ownHolding(new("CAD"), big.NewInt(-30))},
			want: []string{"CAD 70", "USD 7"},
		},
		{
			name: "native leaves out a security with no currency", currency: money.Native,
			rows: []store.Holding{ownHolding(new("CAD"), big.NewInt(100)), ownHolding(nil, big.NewInt(900))},
			want: []string{"CAD 100"},
		},
		{
			name: "native leaves out an unpriced holding alone and gives no total", currency: money.Native,
			rows: []store.Holding{ownHolding(new("CAD"), nil)},
			want: []string{},
		},
		{
			name: "native counts a zero value alone as a zero total", currency: money.Native,
			rows: []store.Holding{ownHolding(new("CAD"), big.NewInt(0)), ownHolding(new("CAD"), nil)},
			want: []string{"CAD 0"},
		},
		{
			name: "cad sums the cad values", currency: money.CAD,
			rows: []store.Holding{
				{ValueCAD: big.NewInt(37_704_00), ValueUSD: big.NewInt(1)},
				{ValueCAD: big.NewInt(33_536_72), ValueUSD: big.NewInt(2)},
			},
			want: []string{"CAD 7124072"},
		},
		{
			name: "usd sums the usd values", currency: money.USD,
			rows: []store.Holding{
				{ValueCAD: big.NewInt(37_704_00), ValueUSD: big.NewInt(1)},
				{ValueCAD: big.NewInt(33_536_72), ValueUSD: big.NewInt(2)},
			},
			want: []string{"USD 3"},
		},
		{
			name: "values with no conversion are left out", currency: money.CAD,
			rows: []store.Holding{cadHolding(big.NewInt(100)), cadHolding(nil), cadHolding(big.NewInt(250))},
			want: []string{"CAD 350"},
		},
		{
			name: "a security quarry does not convert is left out", currency: money.CAD,
			rows: []store.Holding{
				{Currency: new("CAD"), ValueCAD: big.NewInt(100)},
				{Currency: nil, Value: big.NewInt(900)},
				{Currency: new("EUR"), Value: big.NewInt(700)},
			},
			want: []string{"CAD 100"},
		},
		{
			name: "values past the int64 range are summed", currency: money.CAD,
			rows: []store.Holding{cadHolding(big.NewInt(math.MaxInt64)), cadHolding(big.NewInt(math.MaxInt64))},
			want: []string{"CAD 18446744073709551614"},
		},
		{
			name: "a negative value is counted", currency: money.CAD,
			rows: []store.Holding{cadHolding(big.NewInt(500)), cadHolding(big.NewInt(-200))},
			want: []string{"CAD 300"},
		},
		{
			name: "a zero value is the total when it is the only one that converts", currency: money.CAD,
			rows: []store.Holding{cadHolding(big.NewInt(0)), cadHolding(nil)},
			want: []string{"CAD 0"},
		},
		{
			name: "the unconverted currency is listed when no row converts", currency: money.CAD,
			rows: []store.Holding{pricedHolding(new("USD"), 100), pricedHolding(new("USD"), 50)},
			want: []string{"USD 150"},
		},
		{
			name: "the converted currency is listed before the unconverted one", currency: money.CAD,
			rows: []store.Holding{pricedHolding(new("USD"), 100), withValueCAD(pricedHolding(new("CAD"), 300), 300)},
			want: []string{"CAD 300", "USD 100"},
		},
		{
			name: "a usd report lists the cad holdings it could not convert", currency: money.USD,
			rows: []store.Holding{pricedHolding(new("CAD"), 500), withValueUSD(pricedHolding(new("USD"), 200), 200)},
			want: []string{"USD 200", "CAD 500"},
		},
		{
			name: "a priced zero that needs a rate is counted", currency: money.CAD,
			rows: []store.Holding{pricedHolding(new("USD"), 0)},
			want: []string{"USD 0"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, totalValues(holdingsOf(t, c.rows, c.currency)))
		})
	}
}

func Test_holdings_needs_rate_only_for_a_priced_holding_in_the_other_currency_with_no_conversion(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		row      store.Holding
		want     bool
	}{
		{name: "USD in a CAD report", currency: money.CAD, row: pricedHolding(new("USD"), 100), want: true},
		{name: "CAD in a USD report", currency: money.USD, row: pricedHolding(new("CAD"), 100), want: true},
		{name: "CAD in a CAD report", currency: money.CAD, row: pricedHolding(new("CAD"), 100), want: false},
		{name: "USD in a USD report", currency: money.USD, row: pricedHolding(new("USD"), 100), want: false},
		{name: "another currency", currency: money.CAD, row: pricedHolding(new("EUR"), 100), want: false},
		{name: "no currency", currency: money.CAD, row: pricedHolding(nil, 100), want: false},
		{name: "a row a rate converted", currency: money.CAD, row: withValueCAD(pricedHolding(new("USD"), 100), 136), want: false},
		{name: "a holding with no price", currency: money.CAD, row: store.Holding{Currency: new("USD")}, want: false},
		{name: "a native listing", currency: money.Native, row: pricedHolding(new("USD"), 100), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.Holdings{Currency: c.currency}.NeedsRate(c.row))
		})
	}
}

func Test_holdings_refuses_when_a_read_fails_to_open_the_store(t *testing.T) {
	cases := []struct {
		name string
		req  report.HoldingsRequest
	}{
		{name: "the holdings read", req: report.HoldingsRequest{}},
		{name: "the accounts read", req: report.HoldingsRequest{Accounts: []string{"Chequing"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
			srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

			_, err := srv.Holdings(t.Context(), c.req)

			assert.EqualError(t, err, missingStoreRefusal)
		})
	}
}
