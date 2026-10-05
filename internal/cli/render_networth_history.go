package cli

import (
	"math/big"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// renderNetWorthHistory renders n, a history, as one line per month end under one column per account type
// and a Total; a native table adds a Currency column and a line per currency. A cell with no row is blank.
func renderNetWorthHistory(n report.NetWorth) string {
	converted := n.Currency != money.Native
	types := n.Types()
	header, aligns := []string{"Month end"}, []tableAlign{alignLeft}
	if !converted {
		header, aligns = append(header, "Currency"), append(aligns, alignLeft)
	}
	for _, accountType := range types {
		header, aligns = append(header, accountType), append(aligns, alignRight)
	}
	header, aligns = append(header, tableTotalLabel), append(aligns, alignRight)

	rows := [][]string{header}
	for _, date := range n.Dates {
		if converted {
			rows = append(rows, convertedHistoryRow(n, date, types))
			continue
		}
		rows = append(rows, nativeHistoryRows(date, types)...)
	}
	return renderTable(netWorthHistoryCaption(n), aligns, rows)
}

// convertedHistoryRow is date's line: each type's converted sum, then the day's total, blank when no row converts.
func convertedHistoryRow(n report.NetWorth, date report.NetWorthDate, types []string) []string {
	row := []string{date.Date.Format(time.DateOnly)}
	for _, accountType := range types {
		row = append(row, optionalMoney(n.TypeConverted(date, accountType)))
	}
	total := ""
	if len(date.Totals) > 0 {
		total = formatBigMoney(date.Totals[0].Value)
	}
	return append(row, total)
}

// nativeHistoryRows is date's line for each currency it has a row in, each with that currency's total; a day
// with no row gets one line holding only the date.
func nativeHistoryRows(date report.NetWorthDate, types []string) [][]string {
	day := date.Date.Format(time.DateOnly)
	if len(date.Totals) == 0 {
		return [][]string{append([]string{day}, make([]string, len(types)+2)...)}
	}
	rows := make([][]string, len(date.Totals))
	for i, total := range date.Totals {
		row := []string{day, total.Currency}
		for _, accountType := range types {
			row = append(row, optionalMoney(date.TypeBalance(accountType, total.Currency)))
		}
		rows[i] = append(row, formatBigMoney(total.Value))
	}
	return rows
}

// optionalMoney is cents formatted as money, blank when cents is nil.
func optionalMoney(cents *big.Int) string {
	if cents == nil {
		return ""
	}
	return formatBigMoney(cents)
}

// netWorthHistoryCaption names the first and last month end listed, and in CAD or USD the currency of the
// amounts. A history always lists at least one month end: the window's since never exceeds its until.
func netWorthHistoryCaption(n report.NetWorth) string {
	first, last := n.Dates[0].Date, n.Dates[len(n.Dates)-1].Date
	caption := "Net worth at each month end " + first.Format(time.DateOnly) + " to " + last.Format(time.DateOnly)
	if n.Currency != money.Native {
		caption += ", amounts in " + n.Currency.String()
	}
	return caption
}
