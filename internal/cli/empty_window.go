package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// emptyWindowWarning is the warning that window held no activity, subject naming what a command
// reports ("spending"): it says where the transactions it could have counted do run, or that
// there are none. named is whether the command was limited to named accounts, whose
// transactions span is then meant.
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
