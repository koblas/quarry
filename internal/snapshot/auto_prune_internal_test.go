// White-box: listFolder always wraps an OS cause, so a refusal without one
// cannot be produced through a Server.
package snapshot

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errDiskOnFire = errors.New("disk on fire")

func Test_unlistedReason_is_the_reason_of_the_cause_a_folder_refusal_wraps(t *testing.T) {
	t.Parallel()
	refusal := causedRefusalError{msg: "cannot read ~/snapshots: permission denied", cause: &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}}

	assert.Equal(t, "permission denied", unlistedReason(refusal))
}

func Test_unlistedReason_falls_back_to_the_error_itself_when_it_wraps_nothing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "disk on fire", unlistedReason(errDiskOnFire))
}
