package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/report"
)

// recurringAligns is the alignment of the recurring table's columns: Amount and Per year right, the rest left.
var recurringAligns = []tableAlign{
	alignLeft, alignLeft, alignLeft, alignRight, alignRight, alignLeft, alignLeft, alignLeft, alignLeft,
}

// recurringEvery is the Every cell of each cadence.
var recurringEvery = map[report.Cadence]string{
	report.CadenceWeekly:    "week",
	report.CadenceMonthly:   "month",
	report.CadenceQuarterly: "quarter",
	report.CadenceAnnual:    "year",
}

// recurringStatus is the Status cell of each series state.
var recurringStatus = map[report.SeriesState]string{
	report.SeriesActive: "active",
	report.SeriesEnded:  "ended",
}

// recurringNewSuffix follows the state word of a series first charged in the window.
const recurringNewSuffix = ", new"

// statusCell is the Status cell of s: its state, plus the new suffix when it is new.
func statusCell(s report.Series) string {
	if s.New {
		return recurringStatus[s.State] + recurringNewSuffix
	}
	return recurringStatus[s.State]
}

// priceChangesCell is the Price changes cell of s: "N: first -> latest (±p%)", p the change from the
// run's first charge to its latest; empty when the price never moved.
func priceChangesCell(s report.Series) string {
	if len(s.PriceChanges) == 0 {
		return ""
	}
	return fmt.Sprintf("%d: %s -> %s (%s)", len(s.PriceChanges), formatMoney(s.FirstAmount), formatMoney(s.Amount), signedTenths(s.ChangeTenths))
}

// signedTenths renders tenths of a percent as "+11.8%" or "-8.3%"; zero is "0.0%", unsigned.
func signedTenths(tenths int64) string {
	sign := ""
	switch {
	case tenths > 0:
		sign = "+"
	case tenths < 0:
		sign = "-"
		tenths = -tenths
	}
	return fmt.Sprintf("%s%d.%d%%", sign, tenths/10, tenths%10)
}

// renderRecurring renders r as the recurring table: caption, header, one row per series and a Total
// row per currency whose only filled cell is Per year.
func renderRecurring(r report.Recurring) string {
	rows := make([][]string, 0, 1+len(r.Series)+len(r.Totals))
	rows = append(rows, []string{"Payee", "Currency", "Every", "Amount", "Per year", "First", "Last", "Status", "Price changes"})
	for _, s := range r.Series {
		perYear := ""
		if s.PerYear != nil {
			perYear = formatMoney(*s.PerYear)
		}
		rows = append(rows, []string{
			escapeCell(s.Payee), s.Currency, recurringEvery[s.Cadence], formatMoney(s.Amount), perYear,
			s.First.Format(time.DateOnly), s.Last.Format(time.DateOnly), statusCell(s), priceChangesCell(s),
		})
	}
	for _, t := range r.Totals {
		rows = append(rows, []string{tableTotalLabel, t.Currency, "", "", formatMoney(t.PerYear), "", "", "", ""})
	}
	return renderTable(windowCaption("Recurring charges", r.Window, r.Accounts), recurringAligns, rows)
}
