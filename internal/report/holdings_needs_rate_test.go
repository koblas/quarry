package report_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pricedHolding is a priced holding in currency code worth cents in it, with no conversion; a nil code is no currency.
func pricedHolding(code *string, cents int64) store.Holding {
	return store.Holding{Currency: code, Price: new(int64(1_000_000)), Value: big.NewInt(cents)}
}

func Test_holdings_needs_rate_is_true_for_a_priced_holding_in_the_other_currency_with_no_conversion(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		row      store.Holding
	}{
		{name: "USD in a CAD report", currency: money.CAD, row: pricedHolding(new("USD"), 100)},
		{name: "CAD in a USD report", currency: money.USD, row: pricedHolding(new("CAD"), 100)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.True(t, report.Holdings{Currency: c.currency}.NeedsRate(c.row))
		})
	}
}

func Test_holdings_needs_rate_is_false_for_a_row_in_the_reporting_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		code     string
	}{
		{name: "CAD in a CAD report", currency: money.CAD, code: "CAD"},
		{name: "USD in a USD report", currency: money.USD, code: "USD"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.False(t, report.Holdings{Currency: c.currency}.NeedsRate(pricedHolding(new(c.code), 100)))
		})
	}
}

func Test_holdings_needs_rate_is_false_for_a_security_quarry_does_not_convert(t *testing.T) {
	cases := []struct {
		name string
		code *string
	}{
		{name: "another currency", code: new("EUR")},
		{name: "no currency", code: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.False(t, report.Holdings{Currency: money.CAD}.NeedsRate(pricedHolding(c.code, 100)))
		})
	}
}

func Test_holdings_needs_rate_is_false_when_a_rate_converted_the_row(t *testing.T) {
	row := pricedHolding(new("USD"), 100)
	row.ValueCAD = big.NewInt(136)

	assert.False(t, report.Holdings{Currency: money.CAD}.NeedsRate(row))
}

func Test_holdings_needs_rate_is_false_for_a_holding_with_no_price(t *testing.T) {
	assert.False(t, report.Holdings{Currency: money.CAD}.NeedsRate(store.Holding{Currency: new("USD")}))
}

func Test_holdings_needs_rate_is_false_in_a_native_listing(t *testing.T) {
	assert.False(t, report.Holdings{Currency: money.Native}.NeedsRate(pricedHolding(new("USD"), 100)))
}

func Test_holdings_total_lists_the_unconverted_currency_when_no_row_converts(t *testing.T) {
	rows := []store.Holding{pricedHolding(new("USD"), 100), pricedHolding(new("USD"), 50)}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"USD 150"}, totalValues(result))
}

func Test_holdings_total_lists_the_converted_currency_before_the_unconverted_one(t *testing.T) {
	cad := pricedHolding(new("CAD"), 300)
	cad.ValueCAD = big.NewInt(300)
	rows := []store.Holding{pricedHolding(new("USD"), 100), cad}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"CAD 300", "USD 100"}, totalValues(result))
}

func Test_holdings_total_of_a_usd_report_lists_the_cad_holdings_it_could_not_convert(t *testing.T) {
	usd := pricedHolding(new("USD"), 200)
	usd.ValueUSD = big.NewInt(200)
	rows := []store.Holding{pricedHolding(new("CAD"), 500), usd}

	result := holdingsOf(t, rows, money.USD)

	assert.Equal(t, []string{"USD 200", "CAD 500"}, totalValues(result))
}

func Test_holdings_total_counts_a_priced_zero_that_needs_a_rate(t *testing.T) {
	rows := []store.Holding{pricedHolding(new("USD"), 0)}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"USD 0"}, totalValues(result))
}

func Test_holdings_total_has_no_unconverted_entry_for_an_unpriced_holding(t *testing.T) {
	rows := []store.Holding{{Currency: new("USD")}}

	result := holdingsOf(t, rows, money.CAD)

	assert.Empty(t, result.Totals)
}

func Test_holdings_carries_the_first_rate_date_the_store_read(t *testing.T) {
	first := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	srv := report.NewServer(report.WithStore(fakeStore{holdings: store.Holdings{FirstRate: first}}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, first, result.FirstRate)
}
