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

// sourceRefusal classifies a Source failure as encrypted or busy/locked; the
// bool reports whether it recognized err, so a caller that always wants a
// refusal can ignore it.
func sourceRefusal(home, bundlePath string, err error) (bool, error) {
	switch {
	case sqlite.IsNotADB(err):
		return true, RefusalError{msg: homepath.Abbreviate(home, bundlePath) + " is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again"}
	case sqlite.IsBusy(err):
		return true, RefusalError{msg: fmt.Sprintf(
			"Quicken is busy writing %s; run quarry sync again in a moment",
			homepath.Abbreviate(home, bundlePath))}
	default:
		return false, fmt.Errorf("sync %s: %w", bundlePath, err)
	}
}
