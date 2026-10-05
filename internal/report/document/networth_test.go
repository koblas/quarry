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

var netWorthTestDay = time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)

// netWorthJSON is n's document as a map, so a null and an absent key differ.
func netWorthJSON(t *testing.T, n report.NetWorth, warnings []string) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewNetWorth(n, warnings))), &got))
	return got
}

func Test_NewNetWorth_writes_every_key_in_order_with_a_converted_balance_and_a_total(t *testing.T) {
	n := report.NetWorth{
		Dates: []report.NetWorthDate{{
			Date: netWorthTestDay,
			Rows: []store.NetWorthRow{{
				Type: "chequing", Currency: "USD", Balance: big.NewInt(100_000), BalanceCAD: big.NewInt(137_120), BalanceUSD: big.NewInt(100_000),
			}},
			Totals: []report.NetWorthTotal{{Currency: "CAD", Value: big.NewInt(137_120)}},
		}},
		AsOf:     netWorthTestDay,
		Currency: money.CAD,
	}

	got := indented(t, document.NewNetWorth(n, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "as_of": "2026-10-04",
  "since": null,
  "until": null,
  "currency": "CAD",
  "dates": [
    {
      "date": "2026-10-04",
      "balances": [
        {
          "type": "chequing",
          "currency": "USD",
          "balance": "1000.00",
          "converted_balance": "1371.20"
        }
      ],
      "totals": [
        {
          "currency": "CAD",
          "value": "1371.20"
        }
      ]
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewNetWorth_writes_empty_arrays_not_null_for_a_date_with_nothing(t *testing.T) {
	n := report.NetWorth{Dates: []report.NetWorthDate{{Date: netWorthTestDay}}, AsOf: netWorthTestDay, Currency: money.CAD}

	got := netWorthJSON(t, n, nil)

	assert.Equal(t, []any{map[string]any{"date": "2026-10-04", "balances": []any{}, "totals": []any{}}}, got["dates"])
	assert.Equal(t, []any{}, got["warnings"])
}

func Test_NewNetWorth_writes_empty_dates_not_null_when_there_are_none(t *testing.T) {
	got := netWorthJSON(t, report.NetWorth{Currency: money.CAD}, nil)

	assert.Equal(t, []any{}, got["dates"])
}

func Test_NewNetWorth_writes_since_and_until_as_null_and_as_of_as_a_date_in_a_snapshot(t *testing.T) {
	got := netWorthJSON(t, report.NetWorth{AsOf: netWorthTestDay, Currency: money.CAD}, nil)

	assert.Contains(t, got, "since")
	assert.Contains(t, got, "until")
	assert.Nil(t, got["since"])
	assert.Nil(t, got["until"])
	assert.Equal(t, "2026-10-04", got["as_of"])
}

func Test_NewNetWorth_writes_since_and_until_as_dates_and_as_of_as_null_in_a_history(t *testing.T) {
	window := store.Window{Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Until: netWorthTestDay}
	n := report.NetWorth{Window: &window, AsOf: netWorthTestDay, Currency: money.CAD}

	got := netWorthJSON(t, n, nil)

	assert.Contains(t, got, "as_of")
	assert.Nil(t, got["as_of"])
	assert.Equal(t, "2026-01-01", got["since"])
	assert.Equal(t, "2026-10-04", got["until"])
}

func Test_NewNetWorth_keeps_a_month_end_with_no_rows_and_leaves_a_native_converted_balance_null_in_a_history(t *testing.T) {
	window := store.Window{Since: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Until: netWorthTestDay}
	september := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	n := report.NetWorth{
		Dates: []report.NetWorthDate{
			{Date: september},
			{Date: netWorthTestDay, Rows: []store.NetWorthRow{{
				Type: "chequing", Currency: "CAD", Balance: big.NewInt(100), BalanceCAD: big.NewInt(100),
			}}},
		},
		Window:   &window,
		Currency: money.Native,
	}

	got := netWorthJSON(t, n, nil)

	assert.Equal(t, []any{
		map[string]any{"date": "2026-09-30", "balances": []any{}, "totals": []any{}},
		map[string]any{
			"date": "2026-10-04",
			"balances": []any{
				map[string]any{"type": "chequing", "currency": "CAD", "balance": "1.00", "converted_balance": nil},
			},
			"totals": []any{},
		},
	}, got["dates"])
}

func Test_NewNetWorth_keeps_a_zero_balance_row_and_writes_converted_balance_by_listing_currency(t *testing.T) {
	row := store.NetWorthRow{
		Type: "savings", Currency: "CAD", Balance: big.NewInt(0), BalanceCAD: big.NewInt(0), BalanceUSD: big.NewInt(0),
	}
	other := store.NetWorthRow{Type: "chequing", Currency: "CAD", Balance: big.NewInt(100), BalanceCAD: big.NewInt(100)}
	cases := []struct {
		name     string
		currency money.Currency
		want     []any
	}{
		{name: "converted to CAD", currency: money.CAD, want: []any{"0.00", "1.00"}},
		{name: "no rate to USD is null", currency: money.USD, want: []any{"0.00", nil}},
		{name: "native is null", currency: money.Native, want: []any{nil, nil}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := report.NetWorth{Dates: []report.NetWorthDate{{Date: netWorthTestDay, Rows: []store.NetWorthRow{row, other}}}, Currency: c.currency}

			var got struct {
				Dates []struct {
					Balances []struct {
						Converted any `json:"converted_balance"`
					} `json:"balances"`
				} `json:"dates"`
			}
			require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewNetWorth(n, nil))), &got))

			require.Len(t, got.Dates, 1)
			require.Len(t, got.Dates[0].Balances, 2)
			assert.Equal(t, c.want, []any{got.Dates[0].Balances[0].Converted, got.Dates[0].Balances[1].Converted})
		})
	}
}
