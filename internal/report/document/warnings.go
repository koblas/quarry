package document

import (
	"fmt"
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

// SpendingWarnings is s's warnings: one per named account left out, then the unconverted-amounts note,
// the multi-tag-splits note, and a note that the window held no spending.
func SpendingWarnings(s report.Spending, word string) []string {
	warnings := append(leftOutWarnings(s.Accounts, word), unconvertedWarnings(s.Currency, s.Unconverted, transactionsNoun)...)
	if s.By == store.SpendByTag && s.MultiTagSplits > 0 {
		warnings = append(warnings, humanize.Count(s.MultiTagSplits, "split carries", "splits carry")+
			" more than one tag, so the rows add up to more than the total")
	}
	if s.Empty() {
		warnings = appendEmptyWindowWarning(warnings, "spending", s.Accounts, s.Window, s.Transactions)
	}
	return warnings
}

// CashFlowWarnings is c's warnings: one per named account left out,
// then the unconverted-amounts note, then a note that the window held no income or spending.
func CashFlowWarnings(c report.CashFlow, word string) []string {
	warnings := append(leftOutWarnings(c.Accounts, word), unconvertedWarnings(c.Currency, c.Unconverted, transactionsNoun)...)
	if c.Empty() {
		warnings = appendEmptyWindowWarning(warnings, "income or spending", c.Accounts, c.Window, c.Transactions)
	}
	return warnings
}

// RecurringWarnings is r's warnings: one per named account left out of the
// report, then the unconverted-series note, then a note when no series runs in the period.
func RecurringWarnings(r report.Recurring, word string) []string {
	warnings := append(leftOutWarnings(r.Accounts, word), unconvertedWarnings(r.Currency, r.Unconverted, seriesNoun)...)
	if r.Empty() {
		warnings = appendEmptyWindowWarning(warnings, "recurring charges", r.Accounts, r.Window, r.Transactions)
	}
	return warnings
}

// AnomaliesWarnings is a's warnings: one per named account left out of the
// report, then the unconverted-charge note, then a note when no charge was checked in the period.
func AnomaliesWarnings(a report.Anomalies, word string) []string {
	warnings := append(leftOutWarnings(a.Accounts, word), unconvertedWarnings(a.Currency, a.Unconverted, chargesNoun)...)
	if a.Checked == 0 {
		warnings = appendEmptyWindowWarning(warnings, "unusually large charges", a.Accounts, a.Window, a.Transactions)
	}
	return warnings
}

// SearchWarnings is s's warnings: one line when the search matched nothing, naming where the transactions
// searched run (the named accounts' when s names any), and none otherwise. The list is never nil.
func SearchWarnings(s report.Search) []string {
	if s.Matched != 0 {
		return []string{}
	}
	message, owner, have := "no transactions match the search", "the store's", "the store has"
	if len(s.Accounts) > 0 {
		message, owner, have = "no transactions in the named accounts match the search", "their", "they have"
	}
	if s.Transactions == (store.TransactionRange{}) {
		return []string{message + "; " + have + " no transactions"}
	}
	return []string{fmt.Sprintf("%s; %s transactions run %s to %s", message, owner,
		s.Transactions.First.Format(time.DateOnly), s.Transactions.Last.Format(time.DateOnly))}
}

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
		humanize.Count(u.Transactions, noun.singular, noun.plural), u.FirstRate.Format(time.DateOnly), verb, money.NativeOf(currency), currency)
}

// leftOutWarnings is one warning per account in accounts that command leaves out, never nil.
func leftOutWarnings(accounts []store.Account, command string) []string {
	warnings := []string{}
	for _, a := range accounts {
		if !a.LeftOutOfReports() {
			continue
		}
		switch {
		case a.LinkedTracking:
			warnings = append(warnings, linkedTrackingWarning(a, command))
		case a.NotInReports:
			warnings = append(warnings, leftOutOfReportsWarning(a, command))
		}
	}
	return warnings
}

// linkedTrackingWarning is the warning that command counts nothing from a, which uses Quicken's
// linked account tracking.
func linkedTrackingWarning(a store.Account, command string) string {
	return fmt.Sprintf("account %q uses linked account tracking in Quicken, so %s leaves it out, as Quicken's reports do", a.Name, command)
}

// leftOutOfReportsWarning is the warning that command counts nothing from a, which Quicken leaves
// out of reports.
func leftOutOfReportsWarning(a store.Account, command string) string {
	return fmt.Sprintf("account %q is not used in reports in Quicken, so %s leaves it out; "+
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync", a.Name, command)
}

// appendEmptyWindowWarning appends the empty-window note for an empty result, except when every
// account named in accounts is left out: the warnings before it already say why.
func appendEmptyWindowWarning(warnings []string, subject string, accounts []store.Account, window store.Window, span store.TransactionRange) []string {
	named := len(accounts) > 0
	if named && allLeftOut(accounts) {
		return warnings
	}
	return append(warnings, emptyWindowWarning(subject, window, named, span))
}

// allLeftOut reports whether every account in accounts is left out of reports; true for none.
func allLeftOut(accounts []store.Account) bool {
	for _, a := range accounts {
		if !a.LeftOutOfReports() {
			return false
		}
	}
	return true
}

// emptyWindowWarning is the note that window held no subject ("spending"), naming where span's
// transactions run, or that there are none; named means span is the named accounts'.
func emptyWindowWarning(subject string, window store.Window, named bool, span store.TransactionRange) string {
	message := fmt.Sprintf("no %s from %s to %s", subject, window.Since.Format(time.DateOnly), window.Until.Format(time.DateOnly))
	owner, have := "the store's", "the store has"
	if named {
		message += " in the named accounts"
		owner, have = "their", "they have"
	}
	if span == (store.TransactionRange{}) {
		return message + "; " + have + " no transactions"
	}
	return fmt.Sprintf("%s; %s transactions run %s to %s", message, owner,
		span.First.Format(time.DateOnly), span.Last.Format(time.DateOnly))
}
