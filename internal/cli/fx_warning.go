package cli

import (
	"fmt"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// accountsFXWarnings is the warning, if any, that l lists a balance with no rate: the store has no rates,
// or its first rate is dated after today. It is silent unless a listed row shows "no rate".
func accountsFXWarnings(l report.AccountListing) []string {
	if !slices.ContainsFunc(l.Accounts, l.NeedsRate) {
		return nil
	}
	column, other := "In "+l.Currency.String(), money.NativeOf(l.Currency)
	switch {
	case l.FirstRate.IsZero():
		return []string{fmt.Sprintf("the store has no exchange rates, so %s balances show no rate in the %s column; run quarry sync to fetch them", other, column)}
	case l.FirstRate.After(l.AsOf):
		return []string{fmt.Sprintf("the first exchange rate in the store, %s, is dated after today, so %s balances show no rate in the %s column; check the Mac's date and time",
			l.FirstRate.Format(time.DateOnly), other, column)}
	default:
		return nil
	}
}
