// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedClass is one state of the snapshot path the store recorded: reason is why it cannot be
// read, "" when it can, and marked says whether the snapshot of the recorded ID is then the store's.
type recordedClass struct {
	name   string
	reason string
	marked bool
	// recorded is the path the store records for the snapshot with id.
	recorded func(home, id string) string
	// prepare puts the path's parent into the class's state.
	prepare func(t *testing.T, home string)
}

// backupPath is where every class but the first records a snapshot: <home>/Backup/<id>.sqlite.
func backupPath(home, id string) string { return filepath.Join(home, "Backup", id+".sqlite") }

// recordedClasses are the recorded-path states the cells cross: stats, gone, and three ways to fail to stat.
func recordedClasses() []recordedClass {
	return []recordedClass{
		{
			name: "stats", marked: true,
			recorded: func(home, id string) string { return filepath.Join(snapshotsDir(home), id+".sqlite") },
			prepare:  func(*testing.T, string) {},
		},
		{name: "is gone", marked: true, recorded: backupPath, prepare: func(*testing.T, string) {}},
		{
			name: "permission denied", reason: "permission denied", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				skipAsRoot(t)
				backup := filepath.Join(home, "Backup")
				require.NoError(t, os.Mkdir(backup, 0o700))
				require.NoError(t, os.Chmod(backup, 0o000))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
			},
		},
		{
			name: "a symlink loop", reason: "too many levels of symbolic links", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				backup := filepath.Join(home, "Backup")
				require.NoError(t, os.Symlink(backup, backup))
			},
		},
		{
			name: "a parent that is a file", reason: "not a directory", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(home, "Backup"), nil, 0o600))
			},
		},
	}
}

// unreadableRecordedClasses are the classes whose recorded path cannot be read.
func unreadableRecordedClasses() []recordedClass {
	return slices.DeleteFunc(recordedClasses(), func(c recordedClass) bool { return c.reason == "" })
}

// readableRecordedClasses are the classes whose recorded path stats or is gone.
func readableRecordedClasses() []recordedClass {
	return slices.DeleteFunc(recordedClasses(), func(c recordedClass) bool { return c.reason != "" })
}

// buildStoreFromClass builds the store under home from the snapshot with id recorded as c has it, and returns that path.
func buildStoreFromClass(t *testing.T, home, id string, c recordedClass) string {
	t.Helper()
	c.prepare(t, home)
	recorded := c.recorded(home, id)
	buildStoreFrom(t, home, recorded)
	return recorded
}

// cannotReadPhrase is why c's recorded path cannot be read, naming it as shown: "" for a readable class.
func (c recordedClass) cannotReadPhrase(shown string) string {
	if c.reason == "" {
		return ""
	}
	return "cannot read " + shown + ": " + c.reason
}

// pruneRecordedCell runs quarry snapshots prune with args beside five snapshots and a store built from
// pruneMiddle recorded as c has it; it returns the snapshots folder, the recorded path and the run's result.
func pruneRecordedCell(t *testing.T, c recordedClass, args ...string) (string, string, int, string, string) {
	t.Helper()
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	recorded := buildStoreFromClass(t, home, pruneMiddle, c)
	exitCode, stdout, stderr := runPrune(t, args...)
	return dir, recorded, exitCode, stdout, stderr
}

// buildStoreFromUnreadable builds the store under home from <home>/Backup/<id>.sqlite, then makes
// Backup unsearchable, so the recorded path stats with EACCES.
func buildStoreFromUnreadable(t *testing.T, home, id string) {
	t.Helper()
	backup := filepath.Join(home, "Backup")
	require.NoError(t, os.Mkdir(backup, 0o700))
	buildStoreFrom(t, home, filepath.Join(backup, id+".sqlite"))
	require.NoError(t, os.Chmod(backup, 0o000))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
}

func Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFromUnreadable(t, home, pruneMiddle)

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("cannot read ~/Backup/"+pruneMiddle+".sqlite: permission denied"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}
