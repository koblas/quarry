// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chdirToUnsearchableFolder makes the working directory one os.Getwd cannot resolve.
func chdirToUnsearchableFolder(t *testing.T) {
	t.Helper()
	skipAsRoot(t)
	locked := t.TempDir()
	t.Chdir(locked)
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
}

func Test_run_sync_from_a_relative_path_refuses_when_the_working_directory_cannot_be_resolved(t *testing.T) {
	for _, format := range [][]string{nil, {"--json"}} {
		t.Run(strings.Join(format, " "), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			chdirToUnsearchableFolder(t)

			exitCode, stdout, stderr := runSyncFrom(t, "x.sqlite", format...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot resolve x.sqlite against the current folder: permission denied; "+
				"run quarry from a folder you can open\n", stderr)
			assert.NoFileExists(t, storePathUnder(home))
		})
	}
}
