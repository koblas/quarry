package cli_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usdHolding is brokerageHolding priced in USD, 37,704.00 USD, whose CAD value needs an exchange rate the store lacks.
func usdHolding() store.Holding {
	h := brokerageHolding()
	h.Currency, h.ValueCAD, h.ValueUSD = new("USD"), nil, big.NewInt(3_770_400)
	return h
}

// cadHolding is brokerageHolding whose USD value needs an exchange rate the store lacks.
func cadHolding() store.Holding {
	h := brokerageHolding()
	h.ValueUSD = nil
	return h
}

func Test_holdings_in_column_says_no_rate_for_a_priced_cad_or_usd_row_the_other_currency_cannot_convert(t *testing.T) {
	cases := []struct {
		name string
		args []string
		row  store.Holding
	}{
		{name: "USD in CAD", row: usdHolding()},
		{name: "CAD in USD", args: []string{"--currency", "USD"}, row: cadHolding()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{c.row}}}

			err := executeHoldings(t, fake, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Regexp(t, `(?m)^Brokerage +Acme Corp \(ACME\) .* +no rate$`, stdout.String())
		})
	}
}

func Test_holdings_in_column_has_no_no_rate_where_the_row_converts_or_is_not_a_rate_matter(t *testing.T) {
	unpriced := usdHolding()
	unpriced.Price, unpriced.PriceDate, unpriced.Value = nil, nil, nil
	cases := []struct {
		name string
		args []string
		row  store.Holding
	}{
		{name: "a CAD row in CAD", row: brokerageHolding()},
		{name: "a USD row in USD", args: []string{"--currency", "USD"}, row: func() store.Holding {
			h := usdHolding()
			h.ValueUSD = h.Value
			return h
		}()},
		{name: "a CAD row in USD that converts", args: []string{"--currency", "USD"}, row: brokerageHolding()},
		{name: "an unpriced USD row", row: unpriced},
		{name: "a USD row in native", args: []string{"--currency", "native"}, row: usdHolding()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{c.row}}}

			err := executeHoldings(t, fake, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.NotContains(t, stdout.String(), "no rate")
		})
	}
}

func Test_holdings_total_rows_list_the_unconverted_total_below_the_converted_one(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding(), usdHolding()}}}

	err := executeHoldings(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^Total +50\.00\nTotal +USD +37,704\.00\n$`, stdout.String())
}

func Test_holdings_total_rows_list_the_unconverted_total_when_nothing_converts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{usdHolding()}}}

	err := executeHoldings(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^Total +USD +37,704\.00\n$`, stdout.String())
	assert.NotRegexp(t, `(?m)^Total +37,704\.00$`, stdout.String())
}

func Test_holdings_in_usd_total_rows_list_the_cad_total_below_the_usd_one(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding(), cadHolding()}}}

	err := executeHoldings(t, fake, &stdout, &stderr, "--currency", "USD")

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^Total +27\.00\nTotal +CAD +37,704\.00\n$`, stdout.String())
}
