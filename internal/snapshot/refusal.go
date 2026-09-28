package snapshot

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlite"
)

// RefusalError is a final, one-line refusal: its message excludes the
// "quarry: " prefix a caller adds before printing it to stderr.
type RefusalError struct {
	msg string
}

// Error returns the refusal's message verbatim.
func (e RefusalError) Error() string { return e.msg }

// sourceRefusal classifies a Source failure as encrypted or busy/locked; ok
// reports whether it recognized err. An unrecognized err still comes back
// as Sync's ordinary wrapped error, so a caller that always wants a
// sourceRefusal (Open, Probe) can use refusal directly and ignore ok.
func sourceRefusal(home, bundlePath string, err error) (refusal error, ok bool) {
	switch {
	case sqlite.IsNotADB(err):
		return RefusalError{msg: fmt.Sprintf(
			"%s is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again",
			homepath.Abbreviate(home, bundlePath))}, true
	case sqlite.IsBusy(err):
		return RefusalError{msg: fmt.Sprintf(
			"Quicken is busy writing %s; run quarry sync again in a moment",
			homepath.Abbreviate(home, bundlePath))}, true
	default:
		return fmt.Errorf("sync %s: %w", bundlePath, err), false
	}
}
