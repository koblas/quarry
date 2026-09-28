package snapshot

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlite"
)

// errNoAccountsTable is buildManifest's sentinel for a missing ZACCOUNT table.
var errNoAccountsTable = errors.New("no ZACCOUNT table")

// errNoAccounts is buildManifest's sentinel for a ZACCOUNT table with no rows.
var errNoAccounts = errors.New("no accounts")

// contentRefusal classifies a buildManifest failure on the snapshot copy,
// distinct from sourceRefusal's classification of a live-file failure. Any
// unclassified error still refuses in the same shape, naming the bundle and
// the driver's own cause text.
func contentRefusal(home, bundlePath string, err error) error {
	var integrityErr sqlite.IntegrityError
	switch {
	case errors.As(err, &integrityErr):
		return RefusalError{msg: fmt.Sprintf(
			"the snapshot of %s failed SQLite's integrity check (%s); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again",
			homepath.Abbreviate(home, bundlePath), integrityErr.Result)}
	case errors.Is(err, errNoAccountsTable):
		return RefusalError{msg: fmt.Sprintf(
			"%s is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>",
			homepath.Abbreviate(home, bundlePath))}
	case errors.Is(err, errNoAccounts):
		return RefusalError{msg: fmt.Sprintf(
			"%s has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>",
			homepath.Abbreviate(home, bundlePath))}
	default:
		return RefusalError{msg: fmt.Sprintf(
			"cannot read the snapshot of %s: %s; nothing was kept; run quarry sync again",
			homepath.Abbreviate(home, bundlePath), causeText(err))}
	}
}
