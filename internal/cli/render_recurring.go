package cli

import (
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
			s.First.Format(time.DateOnly), s.Last.Format(time.DateOnly), recurringStatus[s.State], "",
		})
	}
	for _, t := range r.Totals {
		rows = append(rows, []string{tableTotalLabel, t.Currency, "", "", formatMoney(t.PerYear), "", "", "", ""})
	}
	return renderTable(windowCaption("Recurring charges", r.Window, nil), recurringAligns, rows)
}
