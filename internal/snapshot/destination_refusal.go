package snapshot

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// causeText unwraps err to its innermost cause's message, falling back to
// err's own text when nothing wraps it further. *fs.PathError and
// *os.LinkError both implement Unwrap, so this one loop classifies either
// without a type switch.
func causeText(err error) string {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err.Error()
		}
		err = unwrapped
	}
}

// unwritableDirRefusal is R13: Destination.Prepare failed, always
// classified as R13 whatever the OS-level cause — its row names the
// directory, not a permission split.
func unwritableDirRefusal(home, snapshotDir string, err error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot write to %s: %s; make the directory writable by your user",
		homepath.Abbreviate(home, snapshotDir), causeText(err))}
}

// writeFaultRefusal classifies a post-Prepare Destination write failure: a
// permission error is still R13; anything else is R14.
func writeFaultRefusal(home, snapshotDir string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return unwritableDirRefusal(home, snapshotDir, err)
	}
	return RefusalError{msg: fmt.Sprintf(
		"cannot write snapshot to %s: %s; free disk space, then run quarry sync again",
		homepath.Abbreviate(home, snapshotDir), causeText(err))}
}
