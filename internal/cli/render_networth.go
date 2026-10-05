package cli

import (
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// renderNetWorth renders n, a snapshot, as the net worth table: caption, header, one row per type and
// currency whose balance is not zero, then one Total row per total. A converted table adds an In column and
// one total in the reporting currency; a native one has no In column and a total per currency.
func renderNetWorth(n report.NetWorth) string {
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
		rows = append(rows, netWorthTotalRow(len(header), converted, total))
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

// netWorthInCell is the balance in the reporting currency, blank when no exchange rate converts it.
func netWorthInCell(n report.NetWorth, row store.NetWorthRow) string {
	value := n.Converted(row)
	if value == nil {
		return ""
	}
	return formatBigMoney(value)
}

// netWorthTotalRow is the Total row of a table width cells wide: a converted table's sum sits in its last
// column, a native one's under Balance with its currency, every other cell blank.
func netWorthTotalRow(width int, converted bool, total report.NetWorthTotal) []string {
	row := make([]string, width)
	row[0] = tableTotalLabel
	if converted {
		row[width-1] = formatBigMoney(total.Value)
		return row
	}
	row[1], row[2] = total.Currency, formatBigMoney(total.Value)
	return row
}
