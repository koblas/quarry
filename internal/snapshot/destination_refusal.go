package snapshot

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// causeText unwraps err to its innermost cause's message, falling back to
// err's own text when nothing wraps it further.
func causeText(err error) string {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err.Error()
		}
		err = unwrapped
	}
}

// unwritableDirRefusal reports that Destination.Prepare failed: it names
// the snapshots directory itself, whatever the OS-level cause, rather than
// splitting on permission versus any other reason.
func unwritableDirRefusal(home, snapshotDir string, err error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot write to %s: %s; make the directory writable by your user",
		homepath.Abbreviate(home, snapshotDir), causeText(err))}
}

// writeFaultRefusal classifies a post-Prepare Destination write failure: a
// permission error reads as an unwritable directory; anything else reads
// as a write fault (e.g. disk full).
func writeFaultRefusal(home, snapshotDir string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return unwritableDirRefusal(home, snapshotDir, err)
	}
	return RefusalError{msg: fmt.Sprintf(
		"cannot write snapshot to %s: %s; free disk space, then run quarry sync again",
		homepath.Abbreviate(home, snapshotDir), causeText(err))}
}
