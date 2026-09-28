package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"

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

// writeFaultRefusal classifies a post-Prepare Destination write failure by
// cause: permission, disk-full/over-quota, or anything else.
func writeFaultRefusal(home, snapshotDir string, err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return unwritableDirRefusal(home, snapshotDir, err)
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return RefusalError{msg: fmt.Sprintf(
			"cannot write snapshot to %s: %s; free disk space, then run quarry sync again",
			homepath.Abbreviate(home, snapshotDir), causeText(err))}
	default:
		return RefusalError{msg: fmt.Sprintf(
			"cannot write snapshot to %s: %s; run quarry sync again",
			homepath.Abbreviate(home, snapshotDir), causeText(err))}
	}
}

// backupFailureRefusal classifies a Destination.Backup failure once
// sourceRefusal found nothing: a write-side cause still reads as a
// destination fault; anything else is presumed a source-copy fault.
func backupFailureRefusal(home, bundlePath, snapshotDir string, err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return writeFaultRefusal(home, snapshotDir, err)
	default:
		return RefusalError{msg: fmt.Sprintf(
			"cannot copy %s: %s; run quarry sync again",
			homepath.Abbreviate(home, bundlePath), causeText(err))}
	}
}
