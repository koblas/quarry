package cli

import (
	"fmt"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// noRatesWarning is the warning for a report that had to convert while the store holds no rates.
const noRatesWarning = "the store has no exchange rates, so amounts are listed in each account's own currency; " +
	"run quarry sync to fetch them"

// countedNoun names what a report counts in its before-the-first-rate line, in the singular and the plural.
type countedNoun struct{ singular, plural string }

// The nouns the reports count: spend and cashflow count transactions, recurring counts series, anomalies count charges.
var (
	transactionsNoun = countedNoun{"transaction", "transactions"}
	chargesNoun      = countedNoun{"charge", "charges"}
	seriesNoun       = countedNoun{"series with a charge", "series with a charge"}
)

// unconvertedWarnings is the warning, if any, that u's counted items were listed in their own currency
// in a report in currency: the no-rates line, or the line counting those before the first rate.
func unconvertedWarnings(currency money.Currency, u store.Unconverted, noun countedNoun) []string {
	switch {
	case u.Transactions == 0:
		return nil
	case u.FirstRate.IsZero():
		return []string{noRatesWarning}
	default:
		return []string{beforeFirstRateWarning(currency, u, noun)}
	}
}

// beforeFirstRateWarning is the line that u.Transactions items of noun, dated before u.FirstRate, are
// listed in the other of CAD and USD than currency.
func beforeFirstRateWarning(currency money.Currency, u store.Unconverted, noun countedNoun) string {
	verb := "are"
	if u.Transactions == 1 {
		verb = "is"
	}
	return fmt.Sprintf("%s dated before %s, the first exchange rate in the store, %s listed in %s, not converted to %s",
		humanize.Count(u.Transactions, noun.singular, noun.plural), u.FirstRate.Format(time.DateOnly), verb, nativeOf(currency), currency)
}

// nativeOf is the currency a CAD or USD report leaves unconverted: the other one.
func nativeOf(currency money.Currency) money.Currency {
	if currency == money.CAD {
		return money.USD
	}
	return money.CAD
}

// accountsFXWarnings is the warning, if any, that l lists a balance with no rate: the store has no rates,
// or its first rate is dated after today. It is silent unless a listed row shows "no rate".
func accountsFXWarnings(l report.AccountListing) []string {
	if !slices.ContainsFunc(l.Accounts, l.NeedsRate) {
		return nil
	}
	column, other := "In "+l.Currency.String(), nativeOf(l.Currency)
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
