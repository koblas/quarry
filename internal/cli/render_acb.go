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

// renderACBYears is the realized-gains table, one row per year with a sale and no total row.
func renderACBYears(a report.ACB) string {
	rows := make([][]string, 0, 1+len(a.Years))
	rows = append(rows, []string{"Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss"})
	for _, year := range a.Years {
		rows = append(rows, []string{
			strconv.Itoa(year.Year), humanize.Thousands(len(year.Sales)), formatMoney(year.Proceeds),
			formatMoney(year.Outlays), formatMoney(year.ACBRemoved), formatMoney(year.Gain),
		})
	}

	return renderTable("Realized capital gains by tax year, in "+money.CAD.String(),
		[]tableAlign{alignLeft, alignRight, alignRight, alignRight, alignRight, alignRight}, rows)
}

// renderACBPositions is the table of each security with shares held on a.AsOf, in the order a lists them.
func renderACBPositions(a report.ACB) string {
	rows := [][]string{{"Security", "Ticker", "Shares", "ACB", "ACB per share"}}
	for _, s := range a.Securities {
		if s.Shares.Sign() <= 0 {
			continue
		}
		ticker := ""
		if s.Security.Ticker != nil {
			ticker = escapeCell(*s.Security.Ticker)
		}
		rows = append(rows, []string{
			escapeCell(s.Security.Name), ticker, formatShares(report.Millionths(s.Shares)),
			formatMoney(s.ACB), formatPerShare(s.PerShare()),
		})
	}

	return renderTable("ACB on "+a.AsOf.Format(time.DateOnly)+", in "+money.CAD.String(),
		[]tableAlign{alignLeft, alignLeft, alignRight, alignRight, alignRight}, rows)
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
