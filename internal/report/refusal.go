package report

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/store"
)

// RefusalError is a final, one-line refusal of a read: its message excludes
// the "quarry: " prefix a caller adds before printing it to stderr, and it
// unwraps to the store error that caused it, if a store error did.
type RefusalError struct {
	msg   string
	cause error
}

// Error returns the refusal's message verbatim.
func (e RefusalError) Error() string { return e.msg }

// Unwrap returns the store error the refusal was made from, or nil when it was not made from one.
func (e RefusalError) Unwrap() error { return e.cause }

// readRefusal is command's refusal of err: interrupted when ctx is done, else the store's refusal.
func (s *Server) readRefusal(ctx context.Context, command string, err error) error {
	if ctx.Err() != nil {
		return RefusalError{msg: command + " interrupted", cause: err}
	}
	return storeRefusal(err, s.home)
}

// storeRefusal turns a *store.OpenError in err into its refusal, naming the store by its ~ form
// under home; any other err, an interrupted query's included, is returned unchanged.
func storeRefusal(err error, home string) error {
	// An interrupted open still carries its OpenError; the interrupt is the refusal.
	if errors.Is(err, store.ErrQueryInterrupted) {
		return err
	}
	openErr, ok := errors.AsType[*store.OpenError](err)
	if !ok {
		return err
	}
	at := homepath.Abbreviate(home, openErr.Path)
	var msg string
	switch openErr.Fault {
	case store.OpenFaultMissing:
		msg = "no store at " + at + " yet; run quarry sync to build it"
	case store.OpenFaultOtherFormat:
		msg = "the store at " + at + " was built by another version of quarry; run quarry sync " + rebuildArgs(openErr.SnapshotPath) + "to rebuild it"
	case store.OpenFaultNotDuckDB, store.OpenFaultPermission, store.OpenFaultOther:
		msg = "cannot read the store at " + at + ": " + openErr.UnreadableReason(at) + "; run quarry sync to rebuild it"
	case store.OpenFaultLocked:
		msg = "cannot read the store at " + at + ": " + openErr.UnreadableReason(at) + "; close that program and run the command again"
	}
	return RefusalError{msg: msg, cause: err}
}

// unknownAccountRefusal refuses an --account value that names no account.
func unknownAccountRefusal(arg string) error {
	return RefusalError{msg: fmt.Sprintf("no account named %q; run quarry accounts --all to list them", arg)}
}

// ambiguousAccountRefusal refuses an --account value naming the accounts with ids, which it lists sorted.
func ambiguousAccountRefusal(arg string, ids []string) error {
	sorted := slices.Sorted(slices.Values(ids))
	return RefusalError{msg: fmt.Sprintf("%d accounts are named %q; pass one of their ids instead: %s", len(ids), arg, strings.Join(sorted, ", "))}
}

// rebuildArgs is the --from argument naming the snapshot at snapshotPath, followed by a space; "" when there is none.
func rebuildArgs(snapshotPath string) string {
	if snapshotPath == "" {
		return ""
	}
	return "--from " + SnapshotID(snapshotPath) + " "
}
