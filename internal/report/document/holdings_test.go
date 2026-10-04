package document_test

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_big_money_renders_cents_of_any_size_ungrouped(t *testing.T) {
	const pastInt64 = "18446744073709551616"
	pastInt64Cents, _ := new(big.Int).SetString(pastInt64, 10)
	cases := []struct {
		name  string
		cents *big.Int
		want  string
	}{
		{name: "zero is 0.00", cents: big.NewInt(0), want: "0.00"},
		{name: "under a unit keeps its leading zero", cents: big.NewInt(5), want: "0.05"},
		{name: "a whole unit", cents: big.NewInt(100), want: "1.00"},
		{name: "a negative under a unit is -0.05, not -1.95", cents: big.NewInt(-5), want: "-0.05"},
		{name: "a negative is not grouped", cents: big.NewInt(-123_456), want: "-1234.56"},
		{name: "a value past int64 keeps every digit", cents: pastInt64Cents, want: "184467440737095516.16"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.BigMoney(c.cents))
		})
	}
}

func holdingsTestRow() store.Holding {
	return store.Holding{
		AccountID: "a-1", Account: "Brokerage", Security: new("Acme Corp"), Ticker: new("ACME"), SecurityID: "s-1",
		Shares: 1_200_000_000, Price: new(int64(31_420_000)), PriceDate: new(time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)),
		Currency: new("CAD"), Value: big.NewInt(3_770_400), ValueCAD: big.NewInt(3_770_400),
	}
}

func Test_NewHoldings_writes_every_key_in_order_with_a_priced_row_and_a_total(t *testing.T) {
	h := report.Holdings{
		Rows:     []store.Holding{holdingsTestRow()},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: big.NewInt(3_770_400)}},
		AsOf:     time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		Currency: money.CAD,
	}

	got := indented(t, document.NewHoldings(h, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "as_of": "2026-10-04",
  "currency": "CAD",
  "account_filter": [],
  "holdings": [
    {
      "account_id": "a-1",
      "account": "Brokerage",
      "account_closed": false,
      "security_id": "s-1",
      "security": "Acme Corp",
      "ticker": "ACME",
      "shares": "1200.000000",
      "price": "31.420000",
      "price_date": "2026-09-29",
      "currency": "CAD",
      "value": "37704.00",
      "converted_value": "37704.00"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "value": "37704.00"
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewHoldings_writes_null_not_empty_for_a_row_with_nothing_to_show(t *testing.T) {
	row := store.Holding{AccountID: "a-1", Account: "Brokerage", AccountClosed: true, SecurityID: "s-1", Shares: -500_000}
	h := report.Holdings{Rows: []store.Holding{row}, Currency: money.CAD}

	var got struct {
		Holdings []map[string]any `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewHoldings(h, nil))), &got))

	require.Len(t, got.Holdings, 1)
	assert.Equal(t, map[string]any{
		"account_id": "a-1", "account": "Brokerage", "account_closed": true, "security_id": "s-1",
		"security": nil, "ticker": nil, "shares": "-0.500000", "price": nil, "price_date": nil,
		"currency": nil, "value": nil, "converted_value": nil,
	}, got.Holdings[0])
}

func Test_NewHoldings_reads_converted_value_from_the_reporting_currency(t *testing.T) {
	row := holdingsTestRow()
	row.Currency, row.ValueCAD, row.ValueUSD = new("USD"), big.NewInt(4_000_000), big.NewInt(3_770_400)
	cases := []struct {
		currency     money.Currency
		wantCurrency string
		want         any
	}{
		{currency: money.CAD, wantCurrency: "CAD", want: "40000.00"},
		{currency: money.USD, wantCurrency: "USD", want: "37704.00"},
		{currency: money.Native, wantCurrency: "native", want: nil},
	}

	for _, c := range cases {
		t.Run(c.currency.String(), func(t *testing.T) {
			var got struct {
				Currency string           `json:"currency"`
				Holdings []map[string]any `json:"holdings"`
			}
			h := report.Holdings{Rows: []store.Holding{row}, Currency: c.currency}

			require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewHoldings(h, nil))), &got))

			assert.Equal(t, c.wantCurrency, got.Currency)
			assert.Equal(t, "37704.00", got.Holdings[0]["value"])
			assert.Equal(t, c.want, got.Holdings[0]["converted_value"])
		})
	}
}

func Test_NewHoldings_writes_a_zero_price_and_a_zero_value_as_amounts_not_null(t *testing.T) {
	zero := holdingsTestRow()
	zero.Price, zero.Value, zero.ValueCAD = new(int64(0)), big.NewInt(0), big.NewInt(0)
	h := report.Holdings{
		Rows:     []store.Holding{zero},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: big.NewInt(0)}},
		Currency: money.CAD,
	}
	var got struct {
		Holdings []map[string]any `json:"holdings"`
		Totals   []map[string]any `json:"totals"`
	}

	require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewHoldings(h, nil))), &got))

	require.Len(t, got.Holdings, 1)
	assert.Equal(t, "0.000000", got.Holdings[0]["price"])
	assert.Equal(t, "0.00", got.Holdings[0]["value"])
	assert.Equal(t, "0.00", got.Holdings[0]["converted_value"])
	assert.Equal(t, []map[string]any{{"currency": "CAD", "value": "0.00"}}, got.Totals)
}

func Test_NewHoldings_writes_empty_lists_not_null_for_no_holdings(t *testing.T) {
	var got map[string]any

	raw := []byte(indented(t, document.NewHoldings(report.Holdings{Currency: money.CAD}, nil)))
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, []any{}, got["holdings"])
	assert.Equal(t, []any{}, got["totals"])
	assert.Equal(t, []any{}, got["account_filter"])
	assert.Equal(t, []any{}, got["warnings"])
}

func Test_NewHoldings_writes_a_value_past_int64_in_full(t *testing.T) {
	past, _ := new(big.Int).SetString("18446744073709551616", 10)
	h := report.Holdings{
		Rows:     []store.Holding{{Value: past, ValueCAD: past}},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: past}},
		Currency: money.CAD,
	}

	got := document.NewHoldings(h, nil)

	assert.Equal(t, "184467440737095516.16", *got.Holdings[0].ConvertedValue)
	assert.Equal(t, "184467440737095516.16", got.Totals[0].Value)
}

func Test_HoldingsWarnings_is_empty_not_nil(t *testing.T) {
	assert.Equal(t, []string{}, document.HoldingsWarnings(report.Holdings{}))
}

func Test_big_money_does_not_change_the_value_it_renders(t *testing.T) {
	cents := big.NewInt(-123_456)

	_ = document.BigMoney(cents)

	assert.Equal(t, "-123456", cents.String())
}
