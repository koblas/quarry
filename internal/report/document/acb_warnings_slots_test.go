package document_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func acbTickered(id, name, ticker string) report.ACBSecurity {
	return report.ACBSecurity{Security: store.Security{ID: id, Name: name, Ticker: &ticker}, Shares: big.NewRat(5, 1)}
}

func Test_ACBWarnings_names_two_securities_that_share_a_ticker(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbTickered("sec-1", "Vanguard Total", "VTI"), acbTickered("sec-2", "Vanguard Total CAD", "VTI")}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"VTI" is 2 securities in Quicken (Vanguard Total, Vanguard Total CAD); ` +
			"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken",
	}, warnings)
}

func Test_ACBWarnings_counts_three_securities_that_share_a_ticker_and_names_each(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbTickered("sec-1", "Alpha", "VTI"), acbTickered("sec-2", "Beta", "VTI"), acbTickered("sec-3", "Gamma", "VTI"),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"VTI" is 3 securities in Quicken (Alpha, Beta, Gamma);`)
}

func Test_ACBWarnings_gives_each_shared_ticker_its_own_line_in_first_member_order(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbTickered("sec-1", "Alpha", "XEQT"), acbTickered("sec-2", "Beta", "VTI"),
		acbTickered("sec-3", "Gamma", "VTI"), acbTickered("sec-4", "Delta", "XEQT"),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"XEQT" is 2 securities in Quicken (Alpha, Delta);`)
	assert.Contains(t, warnings[1], `"VTI" is 2 securities in Quicken (Beta, Gamma);`)
}

// acbSalesOn is a year with one sale on each of the given dates.
func acbSalesOn(year int, dates ...time.Time) report.ACBYear {
	y := report.ACBYear{Year: year}
	for _, date := range dates {
		y.Sales = append(y.Sales, report.ACBSale{Date: date})
	}
	return y
}

func acbDec(year, day int) time.Time {
	return time.Date(year, time.December, day, 0, 0, 0, 0, time.UTC)
}

func Test_ACBWarnings_dates_a_december_sale_warning_by_the_24th_to_the_31st(t *testing.T) {
	cases := []struct {
		name string
		sale time.Time
		want int
	}{
		{name: "november 30 is outside", sale: time.Date(2025, time.November, 30, 0, 0, 0, 0, time.UTC), want: 0},
		{name: "december 23 is outside", sale: acbDec(2025, 23), want: 0},
		{name: "december 24 is inside", sale: acbDec(2025, 24), want: 1},
		{name: "december 31 is inside", sale: acbDec(2025, 31), want: 1},
		{name: "january 1 is outside", sale: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, c.sale)}}

			warnings := document.ACBWarnings(acbPooled(a), acbConfigShown)

			assert.Len(t, warnings, c.want)
		})
	}
}

func Test_ACBWarnings_words_a_december_sale_warning_in_the_singular(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, acbDec(2025, 28))}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown)

	assert.Equal(t, []string{
		"1 sale dated December 24–31, 2025: a sale settles a day or two after its trade date and counts for tax in the year it settles; " +
			"check its date on your T5008",
	}, warnings)
}

func Test_ACBWarnings_counts_the_december_sales_of_one_year_in_one_plural_line(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, acbDec(2025, 24), acbDec(2025, 31), acbDec(2025, 10))}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "2 sales dated December 24–31, 2025:")
}

func Test_ACBWarnings_gives_each_year_with_a_december_sale_its_own_line_oldest_first(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{
		acbSalesOn(2023, acbDec(2023, 29)), acbSalesOn(2024, acbDec(2024, 2)), acbSalesOn(2025, acbDec(2025, 30)),
	}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "1 sale dated December 24–31, 2023:")
	assert.Contains(t, warnings[1], "1 sale dated December 24–31, 2025:")
}

func Test_ACBWarnings_orders_every_slot(t *testing.T) {
	noRate := acbNoRateSecurity("sec-1", "Alpha", "USD", acbDay)
	noRate.Security.Ticker = new("VTI")
	noRate.Events = []report.ACBEvent{
		{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 100, Realized: true},
		acbRemoval("Margin", acbDay, big.NewRat(2, 1)),
		acbNoCostEvent(store.ActionAddShares),
	}
	marked := acbMarkedYear(2025, 1, 1)
	marked.Sales[0].Date = acbDec(2025, 29)
	a := report.ACB{
		Year:             2024,
		FirstRate:        time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
		Years:            []report.ACBYear{marked},
		AdjustmentIssues: []report.ACBAdjustmentIssue{{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: "sec-99", Date: acbDay}},
		Securities:       []report.ACBSecurity{noRate, acbTickered("sec-2", "Beta", "VTI")},
		RegisteredOnly:   []store.Security{{ID: "sec-3", Name: "Gamma"}},
	}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 10)
	assert.Contains(t, warnings[0], "acb.adjustment item 1 names")
	assert.Equal(t, "no sales in 2024 in non-registered accounts; the sales are in 2025", warnings[1])
	assert.Equal(t, `"Gamma" is held only in registered accounts, so it has no ACB`, warnings[2])
	assert.Contains(t, warnings[3], "1 possible superficial loss in 2025:")
	assert.Contains(t, warnings[4], `"Alpha" has shares added with no cost,`)
	assert.Contains(t, warnings[5], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[6], `"Alpha" has a USD trade on`)
	assert.Contains(t, warnings[7], `"VTI" is 2 securities in Quicken`)
	assert.Contains(t, warnings[8], `"Alpha": return of capital on`)
	assert.Contains(t, warnings[9], "1 sale dated December 24–31, 2025:")
}
