package osreason

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

// Reason is err's OS-supplied reason: a path, link or syscall error's own cause,
// else the first line of err, or "unknown error" when that is empty.
func Reason(err error) string {
	reason, _, _ := strings.Cut(err.Error(), "\n")
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		reason = pathErr.Err.Error()
	}
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		reason = linkErr.Err.Error()
	}
	if sysErr, ok := errors.AsType[*os.SyscallError](err); ok {
		reason = sysErr.Err.Error()
	}
	if reason == "" {
		return "unknown error"
	}
	return reason
}
