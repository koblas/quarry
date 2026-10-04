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
	holdingNoPriceCell    = "no price"
	holdingNoCurrencyCell = "none"
	holdingClosedSuffix   = " (closed)"
)

// renderHoldings renders h as the holdings table: caption, header, one row per holding in h's order, and
// a Total row holding only the converted total. The In column is absent in a native listing.
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
	if converted && len(h.Totals) > 0 {
		rows = append(rows, holdingsTotalRow(len(header), formatBigMoney(h.Totals[0].Value)))
	}
	return renderTable(holdingsCaption(h), aligns, rows)
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

// holdingInCell is the value in the reporting currency, blank when the row has none.
func holdingInCell(l report.Holdings, h store.Holding) string {
	if cents := l.Converted(h); cents != nil {
		return formatBigMoney(cents)
	}
	return ""
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
