package osreason

import (
	"errors"
	"io/fs"
	"strings"
)

// Reason is err's OS-supplied reason: a path error's own cause, else the
// first line of err, or "unknown error" when that is empty.
func Reason(err error) string {
	reason, _, _ := strings.Cut(err.Error(), "\n")
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		reason = pathErr.Err.Error()
	}
	if reason == "" {
		return "unknown error"
	}
	return reason
}
