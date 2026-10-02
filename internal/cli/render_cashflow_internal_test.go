// White-box: renderCashFlow and formatRate are unexported layout rules whose column
// widths and rate formats are best driven directly.
package cli

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_renderCashFlow(t *testing.T) {
	cases := []struct {
		name string
		flow report.CashFlow
		want string
	}{
		{
			name: "widths come from the widest cell and a partial period says so in the unpadded Status column",
			flow: report.CashFlow{
				Rows: []report.CashFlowRow{
					{Period: "2026-01", Currency: "CAD", Income: 910000, Spent: 620000, Net: 290000, SavingsRatePct: new(31.9)},
					{Period: "2026-02", Currency: "CAD", Spent: 100000, Net: -100000, Partial: true},
				},
				Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 910000, Spent: 720000, Net: 190000, SavingsRatePct: new(20.9)}},
			},
			want: "" +
				"Cash flow 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Month    Currency    Income     Spent        Net  Savings rate  Status\n" +
				"2026-01  CAD       9,100.00  6,200.00   2,900.00         31.9%\n" +
				"2026-02  CAD           0.00  1,000.00  -1,000.00           n/a  partial\n" +
				"Total    CAD       9,100.00  7,200.00   1,900.00         20.9%\n",
		},
		{
			name: "a year period heads column 1 Year",
			flow: report.CashFlow{
				By:     store.CashFlowByYear,
				Rows:   []report.CashFlowRow{{Period: "2026", Currency: "USD"}},
				Totals: []store.CashFlowTotal{{Currency: "USD"}},
			},
			want: "" +
				"Cash flow 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Year   Currency  Income  Spent   Net  Savings rate  Status\n" +
				"2026   USD         0.00   0.00  0.00           n/a\n" +
				"Total  USD         0.00   0.00  0.00           n/a\n",
		},
		{
			name: "an empty window prints the caption, a blank line and the header only",
			flow: report.CashFlow{},
			want: "" +
				"Cash flow 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Month  Currency  Income  Spent  Net  Savings rate  Status\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.flow.Window = spendingWindow()

			assert.Equal(t, c.want, renderCashFlow(c.flow))
		})
	}
}

func Test_formatRate(t *testing.T) {
	cases := []struct {
		name string
		rate *float64
		want string
	}{
		{name: "no rate is n/a", rate: nil, want: "n/a"},
		{name: "one decimal and a percent sign", rate: new(31.9), want: "31.9%"},
		{name: "zero prints 0.0%", rate: new(0.0), want: "0.0%"},
		{name: "a negative rate keeps its sign", rate: new(-12.5), want: "-12.5%"},
		{name: "a rate that rounds to zero never prints a minus", rate: new(-0.04), want: "0.0%"},
		{name: "the integer part is grouped in thousands", rate: new(-9990.0), want: "-9,990.0%"},
		{name: "a positive rate over a thousand is grouped too", rate: new(1234.5), want: "1,234.5%"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatRate(c.rate))
		})
	}
}

func Test_renderCashFlow_captions_a_converted_report_with_its_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "Cash flow 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n"},
		{name: "USD", currency: money.USD, want: "Cash flow 2026-01-01 to 2026-03-09 in all accounts, amounts in USD\n"},
		{name: "native adds nothing", currency: money.Native, want: "Cash flow 2026-01-01 to 2026-03-09 in all accounts\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flow := report.CashFlow{Window: spendingWindow(), Currency: c.currency}

			assert.True(t, strings.HasPrefix(renderCashFlow(flow), c.want), renderCashFlow(flow))
		})
	}
}
