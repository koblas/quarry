package cli

import (
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// renderNetWorth renders n as a net worth table: a snapshot's rows with their Totals, or a history's month ends.
// Totals that no rate converts print apart from the reporting-currency one.
func renderNetWorth(n report.NetWorth) string {
	if n.Window != nil {
		return renderNetWorthHistory(n)
	}
	converted := n.Currency != money.Native
	header := []string{"Type", "Currency", "Balance"}
	aligns := []tableAlign{alignLeft, alignLeft, alignRight}
	if converted {
		header = append(header, "In "+n.Currency.String())
		aligns = append(aligns, alignRight)
	}

	rows := [][]string{header}
	date := n.Dates[0]
	for _, row := range date.Rows {
		if row.Balance.Sign() == 0 {
			continue
		}
		cells := []string{row.Type, row.Currency, formatBigMoney(row.Balance)}
		if converted {
			cells = append(cells, netWorthInCell(n, row))
		}
		rows = append(rows, cells)
	}
	for _, total := range date.Totals {
		rows = append(rows, netWorthTotalRow(len(header), n, total))
	}
	return renderTable(netWorthCaption(n), aligns, rows)
}

// netWorthCaption names the day and, in CAD or USD, the currency of the amounts.
func netWorthCaption(n report.NetWorth) string {
	caption := "Net worth on " + n.AsOf.Format(time.DateOnly)
	if n.Currency != money.Native {
		caption += ", amounts in " + n.Currency.String()
	}
	return caption
}

// netWorthInCell is the balance in the reporting currency, "no rate" when no exchange rate converts it.
func netWorthInCell(n report.NetWorth, row store.NetWorthRow) string {
	if n.NeedsRate(row) {
		return noRateCell
	}
	return optionalMoney(n.Converted(row))
}

// netWorthTotalRow is the Total row of a table width cells wide: the reporting currency's sum sits in the
// last column, any other currency's under Currency and Balance, every other cell blank.
func netWorthTotalRow(width int, n report.NetWorth, total report.NetWorthTotal) []string {
	row := make([]string, width)
	row[0] = tableTotalLabel
	if n.Currency != money.Native && total.Currency == n.Currency.String() {
		row[width-1] = formatBigMoney(total.Value)
		return row
	}
	row[1], row[2] = total.Currency, formatBigMoney(total.Value)
	return row
}
