package snapshot

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// writeReason unwraps err to its innermost cause's message, falling back
// to err's own text when nothing wraps it further.
func writeReason(err error) string {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err.Error()
		}
		err = unwrapped
	}
}

// prepareRefusal is R13: Destination.Prepare failed, always classified as
// R13 whatever the OS-level cause — its row names the directory, not a
// permission split.
func prepareRefusal(home, snapshotDir string, err error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot write to %s: %s; make the directory writable by your user",
		homepath.Abbreviate(home, snapshotDir), writeReason(err))}
}

// writeRefusal classifies a post-Prepare Destination write failure: a
// permission error is still R13; anything else is R14.
func writeRefusal(home, snapshotDir string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return prepareRefusal(home, snapshotDir, err)
	}
	return RefusalError{msg: fmt.Sprintf(
		"cannot write snapshot to %s: %s; free disk space, then run quarry sync again",
		homepath.Abbreviate(home, snapshotDir), writeReason(err))}
}
