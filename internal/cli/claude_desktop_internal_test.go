// White-box: reportDesktopFailure is unexported and uninstall is not wired to Desktop yet, so its verb
// arms are unreachable through Execute; invalidJSONDetail's non-syntax arm cannot be produced by a file.
package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func Test_reportDesktopFailure_names_the_uninstall_verb_for_a_config_that_is_not_valid_JSON(t *testing.T) {
	err := &claudedesktop.InvalidJSONError{Path: "/c/config.json", Err: errOdd}

	line, got := reportDesktop(uninstallCommand, err)

	require.ErrorIs(t, got, ReportedError{})
	assert.Equal(t, "quarry: claude uninstall: cannot read \"/c/config.json\": it is not valid JSON (odd); "+
		"fix it so Claude Desktop can read it too, then run quarry claude uninstall again\n", line)
}

func Test_reportDesktopFailure_names_the_uninstall_verb_for_a_config_that_cannot_be_read(t *testing.T) {
	err := &claudedesktop.ReadError{Path: "/c/config.json", Err: fs.ErrPermission}

	line, got := reportDesktop(uninstallCommand, err)

	require.ErrorIs(t, got, ReportedError{})
	assert.Equal(t, "quarry: claude uninstall: cannot read \"/c/config.json\" (permission denied); "+
		"check its permissions, then run quarry claude uninstall again\n", line)
}

func Test_reportDesktopFailure_returns_an_unclassified_error_as_a_runtime_error_without_a_report(t *testing.T) {
	line, got := reportDesktop(installCommand, errBoom)

	assert.Equal(t, &runtimeError{err: errBoom}, got)
	assert.Empty(t, line)
}

func Test_invalidJSONDetail_is_empty_for_an_error_without_an_offset(t *testing.T) {
	assert.Empty(t, invalidJSONDetail(errOdd))
}
