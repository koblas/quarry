// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	buildStoreFromUnreadable(t, home, "20260929T090011Z")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, cannotTellPrefix+"cannot read ~/Backup/20260929T090011Z.sqlite: permission denied\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}
