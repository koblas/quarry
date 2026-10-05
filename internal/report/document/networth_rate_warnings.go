package document

import (
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// NativeAdvice is how a surface tells its caller to list balances in their own currencies.
type NativeAdvice string

const (
	// NativeFlag is the advice on the command line, where the currency is a flag.
	NativeFlag NativeAdvice = "pass --currency native"
	// NativeParameter is the advice on the MCP server, where the currency is a tool parameter.
	NativeParameter NativeAdvice = "pass currency native"
)

// rateWarnings is one line when some balance of n needs an exchange rate: that the store has no rates, or that
// the day, or the month ends, listed fall before its first one; none in a native listing or when no balance needs one.
func rateWarnings(n report.NetWorth, advice NativeAdvice) []string {
	needing := datesNeedingRate(n)
	if needing == 0 {
		return nil
	}
	other := money.NativeOf(n.Currency)
	if n.FirstRate.IsZero() {
		return []string{fmt.Sprintf("the store has no exchange rates, so %s balances are not converted to %s and are left out of the %s total; "+
			"%s to list them, or run quarry sync to fetch rates", other, n.Currency, n.Currency, advice)}
	}
	when := n.AsOf.Format(DateLayout) + ","
	if n.Window != nil {
		when = humanize.Count(needing, "month end", "month ends")
	}
	return []string{fmt.Sprintf("%s balances on %s before %s, the first exchange rate in the store, are not converted to %s "+
		"and are left out of the %s total; %s to list them",
		other, when, n.FirstRate.Format(DateLayout), n.Currency, n.Currency, advice)}
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
