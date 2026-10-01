package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// noRatesWarning is the warning for a report that had to convert while the store holds no
// exchange rates.
const noRatesWarning = "the store has no exchange rates, so amounts are listed in each account's own currency; " +
	"run quarry sync to fetch them"

// unconvertedWarnings is the warning, if any, that u's transactions were listed in their own
// currency in a report in currency: none when u counts no transaction, the no-rates line when
// the store has no rate at all, else the line naming how many predate the first rate.
func unconvertedWarnings(currency money.Currency, u store.Unconverted) []string {
	switch {
	case u.Transactions == 0:
		return nil
	case u.FirstRate.IsZero():
		return []string{noRatesWarning}
	default:
		return []string{beforeFirstRateWarning(currency, u)}
	}
}

// beforeFirstRateWarning is the line that u.Transactions transactions, dated before u.FirstRate, are
// listed in the other of CAD and USD than currency.
func beforeFirstRateWarning(currency money.Currency, u store.Unconverted) string {
	verb := "are"
	if u.Transactions == 1 {
		verb = "is"
	}
	return fmt.Sprintf("%s dated before %s, the first exchange rate in the store, %s listed in %s, not converted to %s",
		humanize.Count(u.Transactions, "transaction", "transactions"), u.FirstRate.Format(time.DateOnly), verb, nativeOf(currency), currency)
}

// nativeOf is the currency a CAD or USD report leaves unconverted: the other one.
func nativeOf(currency money.Currency) money.Currency {
	if currency == money.CAD {
		return money.USD
	}
	return money.CAD
}
