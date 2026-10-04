package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const (
	tenUnits         = 10_000_000
	maxDecimal18x6   = 999_999_999_999_999_999
	maxHoldingValue  = "999999999999999998000000.00"
	maxHoldingInUSD  = "799999999999999998400000.00"
	maxHoldingInCAD  = "1249999999999999997500000.00"
	placeholderPrice = "1899-12-29"
)

// quote is a price of millionths for security on date.
func quote(security string, source int64, date time.Time, millionths int64) store.Price {
	return store.Price{SecurityID: security, SourceID: source, Date: date, Price: millionths}
}

func Test_holdings_view_takes_the_latest_price_on_or_before_the_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prices []store.Price
		want   []string
	}{
		{
			name:   "a price dated on the date is used",
			prices: []store.Price{quote(secAcme, 1, marchDay(3), tenUnits), quote(secAcme, 2, marchDay(5), 12_000_000)},
			want:   []string{"12.000000", "2026-03-05", "24.00"},
		},
		{
			name:   "the latest of two earlier prices is used",
			prices: []store.Price{quote(secAcme, 1, marchDay(2), tenUnits), quote(secAcme, 2, marchDay(4), 12_000_000)},
			want:   []string{"12.000000", "2026-03-04", "24.00"},
		},
		{
			name:   "a price dated after the date is not used",
			prices: []store.Price{quote(secAcme, 1, marchDay(3), tenUnits), quote(secAcme, 2, marchDay(6), 99_000_000)},
			want:   []string{"10.000000", "2026-03-03", "20.00"},
		},
		{
			name:   "an old price shows its own day",
			prices: []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)},
			want:   []string{"10.000000", "2026-03-01", "20.00"},
		},
		{
			name:   "another security's price is not used",
			prices: []store.Price{quote(secUSD, 1, marchDay(4), tenUnits)},
			want:   []string{"NULL", "NULL", "NULL"},
		},
		{
			name:   "no price at all leaves price, price date and value NULL",
			prices: nil,
			want:   []string{"NULL", "NULL", "NULL"},
		},
		{
			name:   "a zero price is used as recorded",
			prices: []store.Price{quote(secAcme, 1, marchDay(2), 0)},
			want:   []string{"0.000000", "2026-03-02", "0.00"},
		},
		{
			name:   "the placeholder price date is used as recorded",
			prices: []store.Price{quote(secAcme, 1, day(1899, time.December, 29), tenUnits)},
			want:   []string{"10.000000", placeholderPrice, "20.00"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(buy(acctOne, secAcme, 1, marchDay(1), 2*oneShare))
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, "SELECT price, price_date, value FROM v_holdings WHERE date = '2026-03-05'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_holdings_view_rounds_a_half_cent_away_from_zero_and_below_half_down(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		shares int64
		price  int64
		want   string
	}{
		{name: "exactly half a cent rounds up", shares: 500_000, price: 10_000, want: "0.01"},
		{name: "exactly minus half a cent rounds down", shares: -500_000, price: 10_000, want: "-0.01"},
		{name: "just below half a cent rounds to zero", shares: oneShare, price: 4_999, want: "0.00"},
		{name: "a product that is not whole cents rounds to the nearest", shares: 3_500_000, price: 12_345_678, want: "43.21"},
		{name: "negative shares give a negative value", shares: -2 * oneShare, price: tenUnits, want: "-20.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(buy(acctOne, secAcme, 1, marchDay(1), c.shares))
			rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), c.price)}
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, "SELECT value FROM v_holdings WHERE date = '2026-03-01'")

			assert.Equal(t, [][]string{{c.want}}, got)
		})
	}
}

func Test_holdings_view_values_a_split_day_at_that_days_price(t *testing.T) {
	t.Parallel()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), 10*oneShare),
		splitOf(acctOne, secAcme, 2, marchDay(3), 2, 1))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), tenUnits), quote(secAcme, 2, marchDay(3), 6_000_000)}
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), shares, price, value FROM v_holdings WHERE date IN ('2026-03-02', '2026-03-03') ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-02", "10.000000", "10.000000", "100.00"},
		{"2026-03-03", "20.000000", "6.000000", "120.00"},
	}, got)
}

func Test_holdings_view_values_the_largest_holding_without_overflow(t *testing.T) {
	t.Parallel()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), maxDecimal18x6), buy(acctOne, secUSD, 2, marchDay(1), maxDecimal18x6),
		buy(acctTwo, secAcme, 3, marchDay(1), -maxDecimal18x6), buy(acctTwo, secUSD, 4, marchDay(1), -maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), maxDecimal18x6), quote(secUSD, 2, marchDay(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT account_id, security_id, value, value_cad, value_usd FROM v_holdings WHERE date = '2026-03-01' ORDER BY account_id, security_id")

	assert.Equal(t, [][]string{
		{acctOne, secAcme, maxHoldingValue, maxHoldingValue, maxHoldingInUSD},
		{acctOne, secUSD, maxHoldingValue, maxHoldingInCAD, maxHoldingValue},
		{acctTwo, secAcme, "-" + maxHoldingValue, "-" + maxHoldingValue, "-" + maxHoldingInUSD},
		{acctTwo, secUSD, "-" + maxHoldingValue, "-" + maxHoldingInCAD, "-" + maxHoldingValue},
	}, got)
}

func Test_holdings_view_converts_a_value_at_the_rate_in_force_on_the_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		security string
		account  string
		date     string
		rates    []store.Rate
		want     []string
	}{
		{
			name: "a CAD holding keeps its value and converts to USD", security: secAcme, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "8.00", "1.250000"},
		},
		{
			name: "a USD holding keeps its value and converts to CAD", security: secUSD, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "12.50", "10.00", "1.250000"},
		},
		{
			name: "a Saturday takes the Friday rate", security: secUSD, account: acctOne, date: "2026-03-14", rates: fridayAndMonday(),
			want: []string{"10.00", "12.50", "10.00", "1.250000"},
		},
		{
			name: "a day after the last rate takes the last rate", security: secUSD, account: acctOne, date: "2026-03-20", rates: fridayAndMonday(),
			want: []string{"10.00", "13.00", "10.00", "1.300000"},
		},
		{
			name: "a CAD holding before the first rate keeps its own currency only", security: secAcme, account: acctOne, date: "2026-03-12", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "NULL", "NULL"},
		},
		{
			name: "a USD holding before the first rate keeps its own currency only", security: secUSD, account: acctOne, date: "2026-03-12", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "10.00", "NULL"},
		},
		{
			name: "a CAD holding with no rates in the store keeps its own currency only", security: secAcme, account: acctOne, date: "2026-03-13", rates: nil,
			want: []string{"10.00", "10.00", "NULL", "NULL"},
		},
		{
			name: "a USD holding with no rates in the store keeps its own currency only", security: secUSD, account: acctOne, date: "2026-03-13", rates: nil,
			want: []string{"10.00", "NULL", "10.00", "NULL"},
		},
		{
			name: "a security with no currency converts to nothing and does not borrow its account's", security: secNoCurrency, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "NULL", "1.250000"},
		},
		{
			name: "a security in another currency converts to nothing", security: secEUR, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "NULL", "1.250000"},
		},
		{
			name: "the security's currency wins over its account's", security: secAcme, account: acctTwo, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "8.00", "1.250000"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secUSD, 2, marchDay(1), oneShare),
				buy(acctOne, secEUR, 3, marchDay(1), oneShare), buy(acctOne, secNoCurrency, 4, marchDay(1), oneShare),
				buy(acctTwo, secAcme, 5, marchDay(1), oneShare))
			rows.Prices = []store.Price{
				quote(secAcme, 1, marchDay(1), tenUnits), quote(secUSD, 2, marchDay(1), tenUnits),
				quote(secEUR, 3, marchDay(1), tenUnits), quote(secNoCurrency, 4, marchDay(1), tenUnits),
			}
			st := newStoreWithRates(t, rows, c.rates...)

			got := queryTexts(t, st, "SELECT value, value_cad, value_usd, usd_cad FROM v_holdings WHERE date = '"+c.date+
				"' AND security_id = '"+c.security+"' AND account_id = '"+c.account+"'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}
