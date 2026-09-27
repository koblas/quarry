// White-box: runtimeError is unexported; RunE only unwraps it via
// errors.As, so its Error() and Unwrap() accessors are pinned directly.
package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_runtimeError_reports_the_wrapped_error(t *testing.T) {
	wrapped := &runtimeError{err: errBoom}

	assert.Equal(t, errBoom.Error(), wrapped.Error())
	assert.ErrorIs(t, wrapped, errBoom)
}

var errBoom = errors.New("boom")
