package report

import (
	"cmp"
	"math/big"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// Change is how net worth moved between the listing's first and last dates; nil when the first date has no
// row in any currency. An entry or total is nil-valued when a rate is missing on either day; a type or
// currency absent on one day counts 0 there.
func (n NetWorth) Change() *NetWorthChange {
	if len(n.Dates) == 0 || len(n.Dates[0].Rows) == 0 {
		return nil
	}
	first, last := n.Dates[0], n.Dates[len(n.Dates)-1]
	if n.Currency == money.Native {
		return n.nativeChange(first, last)
	}
	return n.convertedChange(first, last)
}

// convertedChange is the change in the reporting currency; a type or total either day cannot convert is nil.
func (n NetWorth) convertedChange(first, last NetWorthDate) *NetWorthChange {
	code := n.Currency.String()
	change := &NetWorthChange{Types: []NetWorthChangeType{}}
	for _, accountType := range n.Types() {
		entry := NetWorthChangeType{Type: accountType, Currency: code}
		if !n.typeNeedsAnyRate(first, accountType) && !n.typeNeedsAnyRate(last, accountType) {
			entry.Value = difference(n.TypeConverted(last, accountType), n.TypeConverted(first, accountType))
		}
		change.Types = append(change.Types, entry)
	}
	total := NetWorthChangeTotal{Currency: code}
	if from, ok := n.onlyReportingTotal(first); ok {
		if to, ok := n.onlyReportingTotal(last); ok {
			total.Value = difference(to, from)
		}
	}
	change.Totals = []NetWorthChangeTotal{total}
	return change
}

// nativeChange is the change of each currency present on either day, CAD then USD then the rest
// alphabetically, each with the types it holds on either day.
func (n NetWorth) nativeChange(first, last NetWorthDate) *NetWorthChange {
	change := &NetWorthChange{Types: []NetWorthChangeType{}, Totals: []NetWorthChangeTotal{}}
	currencies := nativeCurrencies(first, last)
	for _, code := range currencies {
		for _, accountType := range n.Types() {
			from, to := first.TypeBalance(accountType, code), last.TypeBalance(accountType, code)
			if from != nil || to != nil {
				change.Types = append(change.Types, NetWorthChangeType{Type: accountType, Currency: code, Value: difference(to, from)})
			}
		}
		change.Totals = append(change.Totals, NetWorthChangeTotal{Currency: code, Value: difference(totalIn(last, code), totalIn(first, code))})
	}
	return change
}

// typeNeedsAnyRate reports whether date has any row of accountType that only a missing exchange rate keeps out
// of the converted figure, even when other rows of the type convert.
func (n NetWorth) typeNeedsAnyRate(date NetWorthDate, accountType string) bool {
	return slices.ContainsFunc(date.Rows, func(row store.NetWorthRow) bool { return row.Type == accountType && n.NeedsRate(row) })
}

// onlyReportingTotal is date's total in the reporting currency, 0 for a day without rows; false unless that is
// the day's only total.
func (n NetWorth) onlyReportingTotal(date NetWorthDate) (*big.Int, bool) {
	if len(date.Rows) == 0 {
		return new(big.Int), true
	}
	if len(date.Totals) != 1 || date.Totals[0].Currency != n.Currency.String() {
		return nil, false
	}
	return date.Totals[0].Value, true
}

// nativeCurrencies is the stored currencies of the rows on first and last, CAD then USD then the rest
// alphabetically.
func nativeCurrencies(first, last NetWorthDate) []string {
	codes := make([]string, 0, len(first.Rows)+len(last.Rows))
	for _, row := range slices.Concat(first.Rows, last.Rows) {
		codes = append(codes, row.Currency)
	}
	slices.SortFunc(codes, func(a, b string) int { return cmp.Or(nativeRank(a)-nativeRank(b), strings.Compare(a, b)) })
	return slices.Compact(codes)
}

// totalIn is date's total of one stored currency; nil when the day has no row in it.
func totalIn(date NetWorthDate, code string) *big.Int {
	for _, total := range date.Totals {
		if total.Currency == code {
			return total.Value
		}
	}
	return nil
}

// difference is to - from, a missing end counting 0.
func difference(to, from *big.Int) *big.Int {
	return new(big.Int).Sub(orZero(to), orZero(from))
}

// orZero is value, or 0 when it is nil.
func orZero(value *big.Int) *big.Int {
	if value == nil {
		return new(big.Int)
	}
	return value
}
