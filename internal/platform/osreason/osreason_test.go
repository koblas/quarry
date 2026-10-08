package osreason_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/platform/osreason"
	"github.com/stretchr/testify/assert"
)

func Test_reason_is_the_cause_of_a_path_error_without_the_operation_and_path(t *testing.T) {
	err := &fs.PathError{Op: "open", Path: "/home/x/config.toml", Err: syscall.EACCES}

	assert.Equal(t, "permission denied", osreason.Reason(err))
}

func Test_reason_finds_a_path_error_inside_a_wrapped_error(t *testing.T) {
	err := fmt.Errorf("read config: %w", &fs.PathError{Op: "read", Path: "/x", Err: syscall.EISDIR})

	assert.Equal(t, "is a directory", osreason.Reason(err))
}

func Test_reason_is_the_cause_of_a_syscall_error_without_the_syscall_name(t *testing.T) {
	err := fmt.Errorf("resolve: %w", &os.SyscallError{Syscall: "getwd", Err: syscall.ENOENT})

	assert.Equal(t, "no such file or directory", osreason.Reason(err))
}

func Test_reason_is_the_cause_of_a_link_error_without_the_operation_and_paths(t *testing.T) {
	err := fmt.Errorf("replace /x: %w", &os.LinkError{Op: "rename", Old: "/x/.replace-1", New: "/x/config.json", Err: syscall.EEXIST})

	assert.Equal(t, "file exists", osreason.Reason(err))
}

var (
	errTwoLines = errors.New("disk on fire\nsecond line")
	errBlank    = errors.New("")
)

func Test_reason_is_the_first_line_of_an_error_that_is_not_a_path_error(t *testing.T) {
	assert.Equal(t, "disk on fire", osreason.Reason(errTwoLines))
}

func Test_reason_is_unknown_error_when_the_error_says_nothing(t *testing.T) {
	assert.Equal(t, "unknown error", osreason.Reason(errBlank))
}
