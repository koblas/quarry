package cli

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// acbPerShareDecimals is the decimals of an ACB per share in the position table.
const acbPerShareDecimals = 4

// renderACB renders a as two tables: the gains realized each tax year, one row per year with a sale, then
// the ACB of each security still held on a.AsOf. Both are in CAD.
func renderACB(a report.ACB) string {
	return renderACBYears(a) + "\n" + renderACBPositions(a)
}

// renderACBYears is the realized-gains table, one row per year with a sale or a return of capital above the
// ACB and no total row; a last, unheaded column carries the year's suffixes.
func renderACBYears(a report.ACB) string {
	rows := make([][]string, 0, 1+len(a.Years))
	rows = append(rows, []string{"Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss", ""})
	for _, year := range a.Years {
		rows = append(rows, []string{
			strconv.Itoa(year.Year), humanize.Thousands(len(year.Sales)), formatMoney(year.Proceeds),
			formatMoney(year.Outlays), formatMoney(year.ACBRemoved), formatMoney(year.Gain),
			strings.Join(acbYearSuffixes(year), ", "),
		})
	}

	return renderTable("Realized capital gains by tax year, in "+money.CAD.String(),
		[]tableAlign{alignLeft, alignRight, alignRight, alignRight, alignRight, alignRight, alignLeft}, rows)
}

// acbYearSuffixes are the notes after year's row, in the ruled order.
func acbYearSuffixes(year report.ACBYear) []string {
	var suffixes []string
	if marked := year.PossibleSuperficialLosses(); marked > 0 {
		suffixes = append(suffixes, humanize.Count(marked, "possible superficial loss", "possible superficial losses"))
	}
	if unknown := year.UnknownCostSales(); unknown > 0 {
		suffixes = append(suffixes, humanize.Count(unknown, "sale of shares with unknown cost", "sales of shares with unknown cost"))
	}
	if year.ReturnOfCapitalGain > 0 {
		suffixes = append(suffixes, formatMoney(year.ReturnOfCapitalGain)+" return of capital above ACB, a capital gain")
	}

	return suffixes
}

// renderACBPositions is the table of each security with shares held on a.AsOf, in the order a lists them; a
// last, unheaded column says "incomplete" for a security whose ACB leaves out shares with no recorded cost.
func renderACBPositions(a report.ACB) string {
	rows := [][]string{{"Security", "Ticker", "Shares", "ACB", "ACB per share", ""}}
	for _, s := range a.Securities {
		if s.Shares.Sign() <= 0 {
			continue
		}
		ticker := ""
		if s.Security.Ticker != nil {
			ticker = escapeCell(*s.Security.Ticker)
		}
		note := ""
		if s.Incomplete {
			note = "incomplete"
		}
		rows = append(rows, []string{
			escapeCell(s.Security.Name), ticker, humanize.Shares(report.Millionths(s.Shares)),
			formatMoney(s.ACB), formatPerShare(s.PerShare()), note,
		})
	}

	return renderTable("ACB on "+a.AsOf.Format(time.DateOnly)+", in "+money.CAD.String(),
		[]tableAlign{alignLeft, alignLeft, alignRight, alignRight, alignRight, alignLeft}, rows)
}

// formatPerShare is r, dollars per share, thousands-grouped to acbPerShareDecimals decimals rounded half away
// from zero.
func formatPerShare(r *big.Rat) string {
	fixed := r.FloatString(acbPerShareDecimals)
	sign := ""
	if strings.HasPrefix(fixed, "-") {
		sign, fixed = "-", fixed[1:]
	}
	whole, fraction, _ := strings.Cut(fixed, ".")

	return sign + humanize.ThousandsDigits(whole) + "." + fraction
}
