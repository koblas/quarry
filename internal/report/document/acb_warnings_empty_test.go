package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/stretchr/testify/assert"
)

// acbSoldIn is a pool with one security, asked about the tax year cutTo (0 for the whole report), with a sale in
// each of the years.
func acbSoldIn(cutTo int, years ...int) report.ACB {
	a := acbPooled(report.ACB{Year: cutTo})
	for _, year := range years {
		a.Years = append(a.Years, acbSalesOn(year, time.Date(year, time.June, 1, 0, 0, 0, 0, time.UTC)))
	}
	return a
}

func Test_ACBWarnings_says_why_it_has_nothing_to_show(t *testing.T) {
	const nothingToShow = "no non-registered account has bought or sold a security; quarry acb has nothing to show"
	cases := []struct {
		name string
		a    report.ACB
		want string
	}{
		{name: "no non-registered account traded, whole report", a: report.ACB{}, want: nothingToShow},
		{name: "no non-registered account traded, for a year", a: report.ACB{Year: 2025}, want: nothingToShow},
		{
			name: "the year is empty and so is every other",
			a:    acbSoldIn(2025),
			want: "no sales in 2025 in non-registered accounts, nor in any other year",
		},
		{
			name: "the year is empty and the sales span several",
			a:    acbSoldIn(2023, 2022, 2024, 2026),
			want: "no sales in 2023 in non-registered accounts; the sales are in 2022–2026",
		},
		{
			name: "the year is empty and every sale is in one",
			a:    acbSoldIn(2025, 2022),
			want: "no sales in 2025 in non-registered accounts; the sales are in 2022",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.ACBWarnings(c.a, acbConfigShown))
		})
	}
}

func Test_ACBWarnings_leaves_out_of_the_span_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	a := acbSoldIn(2023, 2022, 2024)
	a.Years = append([]report.ACBYear{{Year: 2020, ReturnOfCapitalGain: 125_000}}, a.Years...)
	a.Years = append(a.Years, report.ACBYear{Year: 2026, ReturnOfCapitalGain: 125_000})

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{"no sales in 2023 in non-registered accounts; the sales are in 2022–2024"}, warnings)
}

func Test_ACBWarnings_is_silent_for_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	a := acbPooled(report.ACB{Year: 2025, Years: []report.ACBYear{{Year: 2025, ReturnOfCapitalGain: 125_000}}})

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_is_silent_when_there_is_something_to_show(t *testing.T) {
	cases := []struct {
		name string
		a    report.ACB
	}{
		{name: "the year has a sale", a: acbSoldIn(2025, 2025)},
		{name: "the whole report is a pool that bought and never sold", a: acbSoldIn(0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, document.ACBWarnings(c.a, acbConfigShown))
		})
	}
}
