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

// renderACB renders a as the gains realized each tax year and the ACB held on a.AsOf, in CAD;
// a report cut to a year prints that year's sales alone, and one naming securities prints their histories.
func renderACB(a report.ACB) string {
	if a.Year != 0 {
		return renderACBSales(a)
	}
	if a.Selected {
		return renderACBHistory(a)
	}

	return renderACBYears(a) + "\n" + renderACBPositions(a)
}

// renderACBSales is the sales of the one year a holds, one row each, then a Total row that is always printed and,
// when the year returned capital above the ACB, a row for that excess. A last, unheaded column carries a sale's marks.
func renderACBSales(a report.ACB) string {
	year := a.Years[0]
	names := make(map[string]string, len(a.Securities))
	for _, s := range a.Securities {
		names[s.Security.ID] = acbSecurityLabel(s)
	}

	rows := [][]string{{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss", ""}}
	for _, sale := range year.Sales {
		rows = append(rows, []string{
			sale.Date.Format(time.DateOnly), names[sale.SecurityID], humanize.Shares(report.Millionths(sale.Shares)),
			formatMoney(sale.Proceeds), formatMoney(sale.Outlays), formatMoney(sale.ACBRemoved), formatMoney(sale.Gain),
			strings.Join(acbSaleMarks(sale), ", "),
		})
	}
	rows = append(rows, []string{
		tableTotalLabel, "", "", formatMoney(year.Proceeds), formatMoney(year.Outlays), formatMoney(year.ACBRemoved), formatMoney(year.Gain), "",
	})
	if year.ReturnOfCapitalGain > 0 {
		rows = append(rows, []string{"", "Return of capital above ACB", "", "", "", "", formatMoney(year.ReturnOfCapitalGain), ""})
	}

	return renderTable("Sales in "+strconv.Itoa(year.Year)+", in "+money.CAD.String(),
		[]tableAlign{alignLeft, alignLeft, alignRight, alignRight, alignRight, alignRight, alignRight, alignLeft}, rows)
}

// acbSecurityLabel is the cell naming s: its ticker, else its name.
func acbSecurityLabel(s report.ACBSecurity) string {
	if s.Security.Ticker != nil && *s.Security.Ticker != "" {
		return escapeCell(*s.Security.Ticker)
	}

	return escapeCell(s.Security.Name)
}

// acbSaleMarks are the notes after a sale's row, in the ruled order.
func acbSaleMarks(sale report.ACBSale) []string {
	var marks []string
	if sale.PossibleSuperficialLoss {
		marks = append(marks, "possible superficial loss")
	}
	if sale.UnknownCost {
		marks = append(marks, "unknown cost")
	}

	return marks
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
		if !s.Holds() {
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
