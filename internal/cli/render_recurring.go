package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
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

// recurringNewSuffix follows the state word of a series first charged in the window.
const recurringNewSuffix = ", new"

// statusCell is the Status cell of s: its state, plus the new suffix when it is new.
func statusCell(s report.Series) string {
	if s.New {
		return document.RecurringStatus(s.State) + recurringNewSuffix
	}
	return document.RecurringStatus(s.State)
}

// priceChangesCell is the Price changes cell of s: "N: first -> latest (±p%)" in the series' own currency,
// each amount prefixed with its code when s is listed in another; empty when the price never moved.
func priceChangesCell(s report.Series) string {
	if len(s.PriceChanges) == 0 {
		return ""
	}
	prefix := ""
	if s.NativeCurrency != s.Currency {
		prefix = s.NativeCurrency + " "
	}
	return fmt.Sprintf("%d: %s%s -> %s%s (%s)", len(s.PriceChanges),
		prefix, formatMoney(s.NativeFirstAmount), prefix, formatMoney(s.NativeAmount), signedTenths(s.ChangeTenths))
}

// currencyCell is the Currency cell of s: the currency it is listed in, with its own currency after it
// in parentheses when the two differ.
func currencyCell(s report.Series) string {
	if s.NativeCurrency != s.Currency {
		return s.Currency + " (" + s.NativeCurrency + ")"
	}
	return s.Currency
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
	return renderRecurringTitled("Recurring charges", r)
}

// renderRecurringTitled is renderRecurring with title as the first words of the caption.
func renderRecurringTitled(title string, r report.Recurring) string {
	rows := make([][]string, 0, 1+len(r.Series)+len(r.Totals))
	rows = append(rows, []string{"Payee", "Currency", "Every", "Amount", "Per year", "First", "Last", "Status", "Price changes"})
	for _, s := range r.Series {
		perYear := ""
		if s.PerYear != nil {
			perYear = formatMoney(*s.PerYear)
		}
		rows = append(rows, []string{
			escapeCell(s.Payee), currencyCell(s), recurringEvery[s.Cadence], formatMoney(s.Amount), perYear,
			s.First.Format(time.DateOnly), s.Last.Format(time.DateOnly), statusCell(s), priceChangesCell(s),
		})
	}
	for _, t := range r.Totals {
		rows = append(rows, []string{tableTotalLabel, t.Currency, "", "", formatMoney(t.PerYear), "", "", "", ""})
	}
	return renderTable(windowCaption(title, r.Window, r.Accounts, r.Currency), recurringAligns, rows)
}
