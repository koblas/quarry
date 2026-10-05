// White-box: renderACB's table rules (years with a sale, positions held, cell escaping) and formatPerShare's
// rounding are unexported cell rules driven directly, not through a command.
package cli

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

var acbDay = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

// acbHeld is a security with shares whole shares held for cents of ACB.
func acbHeld(name string, ticker *string, shares, cents int64) report.ACBSecurity {
	return report.ACBSecurity{Security: store.Security{ID: "sec-" + name, Name: name, Ticker: ticker}, Shares: big.NewRat(shares, 1), ACB: cents}
}

// acbYearOf is year with sales sales, whose totals are the given cents.
func acbYearOf(year, sales int, proceeds, outlays, acbRemoved, gain int64) report.ACBYear {
	return report.ACBYear{
		Year: year, Sales: make([]report.ACBSale, sales),
		Proceeds: proceeds, Outlays: outlays, ACBRemoved: acbRemoved, Gain: gain,
	}
}

func Test_renderACB_prints_a_row_per_year_and_a_row_per_position_held(t *testing.T) {
	a := report.ACB{
		AsOf: acbDay,
		Years: []report.ACBYear{
			acbYearOf(2017, 13, 4_821_055, 8_987, 4_100_210, 711_858),
			acbYearOf(2024, 2, 1_200_400, 999, 1_290_040, -90_639),
		},
		Securities: []report.ACBSecurity{acbHeld("iShares Core Equity ETF", new("XEQT"), 410, 1_041_233)},
	}

	got := renderACB(a)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales   Proceeds  Outlays        ACB  Gain or loss\n"+
		"2017     13  48,210.55    89.87  41,002.10      7,118.58\n"+
		"2024      2  12,004.00     9.99  12,900.40       -906.39\n"+
		"\n"+
		"ACB on 2026-10-05, in CAD\n\n"+
		"Security                 Ticker  Shares        ACB  ACB per share\n"+ //nolint:dupword // the ACB column sits beside the ACB per share column
		"iShares Core Equity ETF  XEQT       410  10,412.33        25.3959\n", got)
}

func Test_renderACB_suffixes_a_years_return_of_capital_gain(t *testing.T) {
	year := acbYearOf(2024, 2, 1_200_400, 999, 1_290_040, -90_639)
	year.ReturnOfCapitalGain = 123_456
	a := report.ACB{AsOf: acbDay, Years: []report.ACBYear{year, acbYearOf(2025, 1, 500, 0, 400, 100)}}

	got := renderACBYears(a)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales   Proceeds  Outlays        ACB  Gain or loss\n"+
		"2024      2  12,004.00     9.99  12,900.40       -906.39  1,234.56 return of capital above ACB, a capital gain\n"+
		"2025      1       5.00     0.00       4.00          1.00\n", got)
}

func Test_renderACB_lists_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	year := report.ACBYear{Year: 2025, ReturnOfCapitalGain: 125_000}

	got := renderACBYears(report.ACB{AsOf: acbDay, Years: []report.ACBYear{year}})

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays   ACB  Gain or loss\n"+
		"2025      0      0.00     0.00  0.00          0.00  1,250.00 return of capital above ACB, a capital gain\n", got)
}

func Test_renderACB_leaves_a_sold_out_security_out_of_the_positions(t *testing.T) {
	a := report.ACB{
		AsOf:       acbDay,
		Securities: []report.ACBSecurity{acbHeld("Sold Fund", new("SLD"), 0, 0), acbHeld("Held Fund", new("HLD"), 3, 600)},
	}

	got := renderACBPositions(a)

	assert.Equal(t, "ACB on 2026-10-05, in CAD\n\n"+
		"Security   Ticker  Shares   ACB  ACB per share\n"+ //nolint:dupword // the ACB column sits beside the ACB per share column
		"Held Fund  HLD          3  6.00         2.0000\n", got)
}

func Test_renderACB_prints_a_fractional_share_count_to_six_decimals_and_a_whole_one_bare(t *testing.T) {
	fractional := func(name string, shares *big.Rat) report.ACBSecurity {
		return report.ACBSecurity{Security: store.Security{ID: "sec-" + name, Name: name}, Shares: shares, ACB: 600}
	}
	a := report.ACB{AsOf: acbDay, Securities: []report.ACBSecurity{
		fractional("Third", big.NewRat(2, 3)), fractional("Half", big.NewRat(5, 2)), fractional("Whole", big.NewRat(4, 1)),
	}}

	got := renderACBPositions(a)

	assert.Equal(t, "ACB on 2026-10-05, in CAD\n\n"+
		"Security  Ticker    Shares   ACB  ACB per share\n"+ //nolint:dupword // the ACB column sits beside the ACB per share column
		"Third             0.666667  6.00         9.0000\n"+
		"Half                   2.5  6.00         2.4000\n"+
		"Whole                    4  6.00         1.5000\n", got)
}

func Test_renderACB_leaves_the_ticker_cell_blank_for_a_security_without_one(t *testing.T) {
	a := report.ACB{AsOf: acbDay, Securities: []report.ACBSecurity{acbHeld("Plain Fund", nil, 2, 1_000)}}

	got := renderACBPositions(a)

	assert.Equal(t, "ACB on 2026-10-05, in CAD\n\n"+
		"Security    Ticker  Shares    ACB  ACB per share\n"+ //nolint:dupword // the ACB column sits beside the ACB per share column
		"Plain Fund               2  10.00         5.0000\n", got)
}

func Test_renderACB_escapes_a_security_name_and_ticker_that_would_break_the_line(t *testing.T) {
	a := report.ACB{AsOf: acbDay, Securities: []report.ACBSecurity{acbHeld("Odd\nFund", new("O\tD"), 1, 100)}}

	got := renderACBPositions(a)

	assert.Equal(t, "ACB on 2026-10-05, in CAD\n\n"+
		"Security   Ticker  Shares   ACB  ACB per share\n"+ //nolint:dupword // the ACB column sits beside the ACB per share column
		`Odd\nFund  O\tD         1  1.00         1.0000`+"\n", got)
}

func Test_renderACB_prints_only_the_headers_for_an_empty_acb(t *testing.T) {
	got := renderACB(report.ACB{AsOf: acbDay})

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\n"+
		"ACB on 2026-10-05, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", got) //nolint:dupword // the ACB column sits beside the ACB per share column
}

func Test_formatPerShare_rounds_half_away_from_zero_and_groups_thousands(t *testing.T) {
	cases := []struct {
		name string
		in   *big.Rat
		want string
	}{
		{name: "a terminating value keeps four decimals", in: big.NewRat(5, 2), want: "2.5000"},
		{name: "a half in the fifth decimal rounds up", in: big.NewRat(1, 20_000), want: "0.0001"},
		{name: "under a half in the fifth decimal rounds down", in: big.NewRat(1, 30_000), want: "0.0000"},
		{name: "a repeating value rounds", in: big.NewRat(32, 3), want: "10.6667"},
		{name: "four digits are grouped", in: big.NewRat(12_345, 10), want: "1,234.5000"},
		{name: "a negative is signed and grouped", in: big.NewRat(-12_345, 10), want: "-1,234.5000"},
		{name: "a negative half rounds away from zero", in: big.NewRat(-1, 20_000), want: "-0.0001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatPerShare(c.in))
		})
	}
}
