// White-box: reportDesktopFailure's unclassified arm needs an error no Desktop operation returns, and
// invalidJSONDetail's non-syntax arm cannot be produced by a file.
package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var (
	errOdd  = errors.New("odd")
	errBoom = errors.New("boom")
)

func reportDesktop(verb string, err error) (string, error) {
	var errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&errOut)
	got := reportDesktopFailure(cmd, verb, "", err)
	return errOut.String(), got
}

func Test_reportDesktopFailure_returns_an_unclassified_error_as_a_runtime_error_without_a_report(t *testing.T) {
	line, got := reportDesktop(installCommand, errBoom)

	assert.Equal(t, &runtimeError{err: errBoom}, got)
	assert.Empty(t, line)
}

func Test_invalidJSONDetail_is_empty_for_an_error_without_an_offset(t *testing.T) {
	assert.Empty(t, invalidJSONDetail(errOdd))
}
