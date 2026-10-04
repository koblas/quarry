// White-box: holdingCells, formatPrice and formatBigMoney are unexported cell rules whose
// edge values (price decimals, a value past int64) are driven directly, not through a table.
package cli

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

var holdingsDay = time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)

// heldRow is 10 shares of "Fund" in CAD priced 5.00 on 2026-03-05, worth 50.00 in CAD; tests change the field they pin.
func heldRow() store.Holding {
	return store.Holding{
		Account: "Brokerage", Security: new("Fund"), Shares: 10_000_000,
		Price: new(int64(5_000_000)), PriceDate: new(time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC)),
		Currency: new("CAD"), Value: big.NewInt(5_000), ValueCAD: big.NewInt(5_000), ValueUSD: big.NewInt(3_700),
	}
}

// holdingsIn is the listing of rows in currency with the totals given, none when totals is nil.
func holdingsIn(currency money.Currency, totals []report.HoldingsTotal, rows ...store.Holding) report.Holdings {
	return report.Holdings{Rows: rows, Totals: totals, AsOf: holdingsDay, Currency: currency}
}

// totalIn is the one total of cents in currency.
func totalIn(currency money.Currency, cents int64) []report.HoldingsTotal {
	return []report.HoldingsTotal{{Currency: currency.String(), Value: big.NewInt(cents)}}
}

func Test_holdingCells_account_cell(t *testing.T) {
	cases := []struct {
		name    string
		account string
		closed  bool
		want    string
	}{
		{name: "an open account is its name", account: "Brokerage", want: "Brokerage"},
		{name: "a closed account gets a closed suffix", account: "Old RRSP", closed: true, want: "Old RRSP (closed)"},
		{name: "a newline in the name is escaped", account: "Joint\nAccount", want: `Joint\nAccount`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := heldRow()
			row.Account, row.AccountClosed = c.account, c.closed

			assert.Equal(t, c.want, holdingCells(row)[0])
		})
	}
}

func Test_holdingCells_security_cell(t *testing.T) {
	cases := []struct {
		name     string
		security *string
		ticker   *string
		want     string
	}{
		{name: "no ticker shows the name", security: new("Maple Fund"), ticker: nil, want: "Maple Fund"},
		{name: "a ticker equal to the name is not repeated", security: new("Maple Fund"), ticker: new("Maple Fund"), want: "Maple Fund"},
		{name: "a ticker that differs follows the name", security: new("Acme Corp"), ticker: new("ACME"), want: "Acme Corp (ACME)"},
		{name: "a newline in the name is escaped", security: new("Acme\nCorp"), ticker: nil, want: `Acme\nCorp`},
		{name: "a missing security row is blank", security: nil, ticker: nil, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := heldRow()
			row.Security, row.Ticker = c.security, c.ticker

			assert.Equal(t, c.want, holdingCells(row)[1])
		})
	}
}

func Test_holdingCells_currency_cell(t *testing.T) {
	cases := []struct {
		name     string
		currency *string
		want     string
	}{
		{name: "the security's own currency", currency: new("CAD"), want: "CAD"},
		{name: "none when the security has no currency", currency: nil, want: "none"},
		{name: "a newline in the code is escaped", currency: new("C\nAD"), want: `C\nAD`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := heldRow()
			row.Currency = c.currency

			assert.Equal(t, c.want, holdingCells(row)[5])
		})
	}
}

func Test_formatPrice(t *testing.T) {
	cases := []struct {
		name       string
		millionths int64
		want       string
	}{
		{name: "trailing zeros are trimmed", millionths: 31_420_000, want: "31.42"},
		{name: "a whole price gets two decimals", millionths: 5_000_000, want: "5.00"},
		{name: "one decimal is padded to two", millionths: 1_500_000, want: "1.50"},
		{name: "six decimals are kept", millionths: 12_345_678, want: "12.345678"},
		{name: "zero is 0.00", millionths: 0, want: "0.00"},
		{name: "thousands are grouped", millionths: 1_234_500_000, want: "1,234.50"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatPrice(c.millionths))
		})
	}
}

func Test_formatBigMoney(t *testing.T) {
	pastInt64, _ := new(big.Int).SetString("18446744073709551616", 10)
	cases := []struct {
		name  string
		cents *big.Int
		want  string
	}{
		{name: "zero is 0.00", cents: big.NewInt(0), want: "0.00"},
		{name: "a small negative keeps its sign", cents: big.NewInt(-5), want: "-0.05"},
		{name: "a negative is grouped", cents: big.NewInt(-123_456), want: "-1,234.56"},
		{name: "a value past int64 is grouped whole", cents: pastInt64, want: "184,467,440,737,095,516.16"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatBigMoney(c.cents))
		})
	}
}

func Test_renderHoldings_a_holding_with_no_price_has_no_value_and_is_not_in_the_total(t *testing.T) {
	unpriced := heldRow()
	unpriced.Account, unpriced.Price, unpriced.PriceDate, unpriced.Value, unpriced.ValueCAD = "Unpriced", nil, nil, nil, nil
	priced := heldRow()

	got := renderHoldings(holdingsIn(money.CAD, totalIn(money.CAD, 5_000), unpriced, priced))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account    Security  Shares     Price  Priced on   Currency  Value  In CAD\n"+
		"Unpriced   Fund          10  no price              CAD\n"+
		"Brokerage  Fund          10      5.00  2026-03-05  CAD       50.00   50.00\n"+
		"Total                                                                50.00\n", got)
}

func Test_renderHoldings_a_zero_price_is_shown_and_its_zero_value_is_the_total(t *testing.T) {
	zero := heldRow()
	zero.Price, zero.Value, zero.ValueCAD = new(int64(0)), big.NewInt(0), big.NewInt(0)

	got := renderHoldings(holdingsIn(money.CAD, totalIn(money.CAD, 0), zero))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account    Security  Shares  Price  Priced on   Currency  Value  In CAD\n"+
		"Brokerage  Fund          10   0.00  2026-03-05  CAD        0.00    0.00\n"+
		"Total                                                              0.00\n", got)
}

func Test_renderHoldings_negative_shares_give_a_negative_value_that_is_in_the_total(t *testing.T) {
	short := heldRow()
	short.Shares, short.Value, short.ValueCAD = -10_000_000, big.NewInt(-5_000), big.NewInt(-5_000)

	got := renderHoldings(holdingsIn(money.CAD, totalIn(money.CAD, -5_000), short))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account    Security  Shares  Price  Priced on   Currency   Value  In CAD\n"+
		"Brokerage  Fund         -10   5.00  2026-03-05  CAD       -50.00  -50.00\n"+
		"Total                                                             -50.00\n", got)
}

func Test_renderHoldings_the_total_row_holds_only_the_label_and_the_in_sum_for_rows_in_two_currencies(t *testing.T) {
	cad := heldRow()
	usd := heldRow()
	usd.Account, usd.Currency, usd.Value, usd.ValueUSD, usd.ValueCAD = "IRA", new("USD"), big.NewInt(1_000), big.NewInt(1_000), big.NewInt(1_360)

	got := renderHoldings(holdingsIn(money.CAD, totalIn(money.CAD, 6_360), cad, usd))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account    Security  Shares  Price  Priced on   Currency  Value  In CAD\n"+
		"Brokerage  Fund          10   5.00  2026-03-05  CAD       50.00   50.00\n"+
		"IRA        Fund          10   5.00  2026-03-05  USD       10.00   13.60\n"+
		"Total                                                             63.60\n", got)
}

func Test_renderHoldings_shows_an_old_price_and_the_placeholder_price_date_as_recorded(t *testing.T) {
	old := heldRow()
	old.PriceDate = new(time.Date(1899, time.December, 29, 0, 0, 0, 0, time.UTC))

	got := renderHoldings(holdingsIn(money.CAD, totalIn(money.CAD, 5_000), old))

	assert.Contains(t, got, "Brokerage  Fund          10   5.00  1899-12-29  CAD       50.00   50.00\n")
}

func Test_renderHoldings_names_the_reporting_currency_in_the_caption_and_the_column(t *testing.T) {
	got := renderHoldings(holdingsIn(money.USD, totalIn(money.USD, 3_700), heldRow()))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in USD; cash not included\n\n"+
		"Account    Security  Shares  Price  Priced on   Currency  Value  In USD\n"+
		"Brokerage  Fund          10   5.00  2026-03-05  CAD       50.00   37.00\n"+
		"Total                                                             37.00\n", got)
}

func Test_renderHoldings_a_native_listing_has_no_in_column_and_no_amounts_in_caption(t *testing.T) {
	got := renderHoldings(holdingsIn(money.Native, nil, heldRow()))

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts; cash not included\n\n"+
		"Account    Security  Shares  Price  Priced on   Currency  Value\n"+
		"Brokerage  Fund          10   5.00  2026-03-05  CAD       50.00\n", got)
}
