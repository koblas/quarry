package document

import (
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// rateWarnings is one line when some balance of n needs an exchange rate: that the store has no rates, or that
// the day, or the month ends, listed fall before its first one; none in a native listing or when no balance needs one.
func rateWarnings(n report.NetWorth) []string {
	needing := datesNeedingRate(n)
	if needing == 0 {
		return nil
	}
	other := money.NativeOf(n.Currency)
	if n.FirstRate.IsZero() {
		return []string{fmt.Sprintf("the store has no exchange rates, so %s balances are not converted to %s and are left out of the %s total; "+
			"pass --currency native to list them, or run quarry sync to fetch rates", other, n.Currency, n.Currency)}
	}
	when := n.AsOf.Format(DateLayout) + ","
	if n.Window != nil {
		when = humanize.Count(needing, "month end", "month ends")
	}
	return []string{fmt.Sprintf("%s balances on %s before %s, the first exchange rate in the store, are not converted to %s "+
		"and are left out of the %s total; pass --currency native to list them",
		other, when, n.FirstRate.Format(DateLayout), n.Currency, n.Currency)}
}

// datesNeedingRate is how many of n's listed days have a balance that needs an exchange rate.
func datesNeedingRate(n report.NetWorth) int {
	var count int
	for _, date := range n.Dates {
		if slices.ContainsFunc(date.Rows, n.NeedsRate) {
			count++
		}
	}
	return count
}
