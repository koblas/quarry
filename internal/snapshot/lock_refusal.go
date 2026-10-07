package snapshot

import (
	"path/filepath"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/platform/osreason"
)

// lockRefusal phrases a classified lock failure as a RefusalError; false means it has no copy for
// err: an unknown kind, or a kind that quotes an OS reason and has no cause to quote.
func lockRefusal(home string, err *lockfile.Error, words lockWords) (RefusalError, bool) {
	lock := homepath.Abbreviate(home, err.Path)
	folder := homepath.Abbreviate(home, filepath.Dir(err.Path))
	switch {
	case err.Kind == lockfile.KindHeld:
		return RefusalError{msg: words.held}, true
	case err.Kind == lockfile.KindNotRegular:
		return RefusalError{msg: lock + " is not a regular file; remove it, then run the command again"}, true
	case err.Kind == lockfile.KindNotFolder:
		return RefusalError{msg: folder + " is not a folder; rename or remove it, then run the command again"}, true
	case err.Err == nil:
		return RefusalError{}, false
	}
	reason := osreason.Reason(err.Err)
	switch err.Kind {
	case lockfile.KindFolderCreate:
		parent := homepath.Abbreviate(home, filepath.Dir(filepath.Dir(err.Path)))
		return RefusalError{msg: "cannot create " + folder + ": " + reason + "; make " + parent + " writable by your user, then run the command again"}, true
	case lockfile.KindCreate:
		return RefusalError{msg: "cannot create " + lock + ": " + reason + "; make " + folder + " writable by your user, then run the command again"}, true
	case lockfile.KindOpen:
		return RefusalError{msg: "cannot open " + lock + ": " + reason + "; make it readable by your user, or remove it, then run the command again"}, true
	case lockfile.KindFolderOpen:
		return RefusalError{msg: "cannot open " + folder + ": " + reason + "; make it readable and writable by your user, then run the command again"}, true
	case lockfile.KindLock:
		return RefusalError{msg: "cannot lock " + lock + ": " + reason + ", so " + words.outcome + "; " + folder + " must be on a disk that supports file locks"}, true
	case lockfile.KindHeld, lockfile.KindNotRegular, lockfile.KindFolderMissing, lockfile.KindNotFolder:
	}
	return RefusalError{}, false
}
