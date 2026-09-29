package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// leftOutWarnings is one warning per account in accounts that Quicken leaves out of reports,
// in order, saying command leaves it out; never nil.
func leftOutWarnings(accounts []store.Account, command string) []string {
	warnings := []string{}
	for _, a := range accounts {
		if a.NotInReports {
			warnings = append(warnings, leftOutOfReportsWarning(a, command))
		}
	}
	return warnings
}

// leftOutOfReportsWarning is the warning that command counts nothing from a, which Quicken leaves
// out of reports.
func leftOutOfReportsWarning(a store.Account, command string) string {
	return fmt.Sprintf("account %q is not used in reports in Quicken, so %s leaves it out; "+
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync", a.Name, command)
}

// appendEmptyWindowWarning appends the empty-window note for an empty result, except when every
// account named in accounts is left out of reports: the warnings before it already say why.
func appendEmptyWindowWarning(warnings []string, subject string, accounts []store.Account, window store.Window, span store.TransactionRange) []string {
	named := len(accounts) > 0
	leftOut := 0
	for _, a := range accounts {
		if a.NotInReports {
			leftOut++
		}
	}
	if named && leftOut == len(accounts) {
		return warnings
	}
	return append(warnings, emptyWindowWarning(subject, window, named, span))
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
