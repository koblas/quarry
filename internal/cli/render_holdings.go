package cli

import (
	"math/big"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// The holdings table's cells for a row with nothing to show there.
const (
	holdingNoPriceCell      = "no price"
	holdingNoCurrencyCell   = "none"
	holdingNotConvertedCell = "not converted"
	holdingNoRateCell       = "no rate"
	holdingClosedSuffix     = " (closed)"
)

// The Currency and Value columns' positions in a holdings table row, the same in a native and a converted table.
const (
	holdingCurrencyColumn = 5
	holdingValueColumn    = 6
)

// renderHoldings renders h as the holdings table: caption, header, one row per holding in h's order, and
// one Total row per total. The reporting currency's total sits in the In column; every other total sits
// under Value with its currency. A native listing has no In column.
func renderHoldings(h report.Holdings) string {
	converted := h.Currency != money.Native
	header := []string{"Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value"}
	aligns := []tableAlign{alignLeft, alignLeft, alignRight, alignRight, alignLeft, alignLeft, alignRight}
	if converted {
		header = append(header, "In "+h.Currency.String())
		aligns = append(aligns, alignRight)
	}

	rows := [][]string{header}
	for _, holding := range h.Rows {
		row := holdingCells(holding)
		if converted {
			row = append(row, holdingInCell(h, holding))
		}
		rows = append(rows, row)
	}
	for _, total := range h.Totals {
		if converted && total.Currency == h.Currency.String() {
			rows = append(rows, holdingsTotalRow(len(header), formatBigMoney(total.Value)))
		} else {
			rows = append(rows, holdingsCurrencyTotalRow(len(header), total))
		}
	}
	return renderTable(holdingsCaption(h), aligns, rows)
}

// holdingsCurrencyTotalRow is the Total row of one currency in a table width cells wide: the label, the
// currency in the Currency column and the sum in the Value column, every other cell blank.
func holdingsCurrencyTotalRow(width int, total report.HoldingsTotal) []string {
	row := make([]string, width)
	row[0] = tableTotalLabel
	row[holdingCurrencyColumn] = total.Currency
	row[holdingValueColumn] = formatBigMoney(total.Value)
	return row
}

// holdingsCaption names the day, the accounts and, in CAD or USD, the currency of the amounts.
func holdingsCaption(h report.Holdings) string {
	caption := "Holdings on " + h.AsOf.Format(time.DateOnly) + " in " + accountsCaption(nil)
	if h.Currency != money.Native {
		caption += ", amounts in " + h.Currency.String()
	}
	return caption + "; cash not included"
}

// holdingsTotalRow is the Total row of a table width cells wide: the label and sum, every other cell blank.
func holdingsTotalRow(width int, sum string) []string {
	row := make([]string, width)
	row[0] = tableTotalLabel
	row[width-1] = sum
	return row
}

// holdingCells is the cells of one holding up to and including Value.
func holdingCells(h store.Holding) []string {
	return []string{
		escapeCell(h.Account) + holdingClosed(h),
		holdingSecurity(h),
		formatShares(h.Shares),
		holdingPrice(h),
		holdingPricedOn(h),
		holdingCurrency(h),
		holdingValue(h),
	}
}

// holdingClosed is " (closed)" for a holding in a closed account, else "".
func holdingClosed(h store.Holding) string {
	if h.AccountClosed {
		return holdingClosedSuffix
	}
	return ""
}

// holdingSecurity is the security's label, blank when the holding's security row is missing.
func holdingSecurity(h store.Holding) string {
	if h.Security == nil {
		return ""
	}
	return securityLabel(*h.Security, h.Ticker)
}

// holdingPrice is the price per share, "no price" when none is recorded on or before the day.
func holdingPrice(h store.Holding) string {
	if h.Price == nil {
		return holdingNoPriceCell
	}
	return formatPrice(*h.Price)
}

// holdingPricedOn is the day of the price, blank when there is none.
func holdingPricedOn(h store.Holding) string {
	if h.PriceDate == nil {
		return ""
	}
	return h.PriceDate.Format(time.DateOnly)
}

// holdingCurrency is the security's own currency, "none" when it has none.
func holdingCurrency(h store.Holding) string {
	if h.Currency == nil {
		return holdingNoCurrencyCell
	}
	return escapeCell(*h.Currency)
}

// holdingValue is the value in the security's own currency, blank when there is no price.
func holdingValue(h store.Holding) string {
	if h.Value == nil {
		return ""
	}
	return formatBigMoney(h.Value)
}

// holdingInCell is the value in the reporting currency: blank with no price, "not converted" for a
// priced security quarry cannot convert, "no rate" for one only a missing exchange rate keeps unconverted.
// Every other priced row has a converted value: a row in the reporting currency needs no rate.
func holdingInCell(l report.Holdings, h store.Holding) string {
	if h.Price == nil {
		return ""
	}
	if !report.Convertible(h) {
		return holdingNotConvertedCell
	}
	if l.NeedsRate(h) {
		return holdingNoRateCell
	}
	return formatBigMoney(l.Converted(h))
}

// formatPrice renders millionths of a currency unit like formatShares, with at least two decimals.
func formatPrice(millionths int64) string {
	s := formatShares(millionths)
	_, decimals, found := strings.Cut(s, ".")
	if !found {
		return s + ".00"
	}
	if len(decimals) < 2 {
		return s + "0"
	}
	return s
}

// formatBigMoney renders cents, of any size, thousands-grouped with 2 decimals and a leading "-" for a negative value.
func formatBigMoney(cents *big.Int) string {
	sign, unsigned := "", document.BigMoney(cents)
	if cents.Sign() < 0 {
		sign, unsigned = "-", unsigned[1:]
	}
	whole, fraction, _ := strings.Cut(unsigned, ".")
	return sign + humanize.ThousandsDigits(whole) + "." + fraction
}
