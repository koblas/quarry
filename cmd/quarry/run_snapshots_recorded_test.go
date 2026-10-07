// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedID is the snapshot the recorded-path cells record: the newer of olderPair's two.
const recordedID = "20260929T090011Z"

// cannotTellSentence is the sentence that the store's snapshot cannot be told, naming c's recorded
// path as shown; "" for a readable class.
func (c recordedClass) cannotTellSentence(shown string) string {
	if c.reason == "" {
		return ""
	}
	return "cannot tell which snapshot the store was built from: " + c.cannotReadPhrase(shown)
}

// stderrWarning is what snapshots prints to stderr for c: the warning line, or nothing.
func (c recordedClass) stderrWarning() string {
	if c.reason == "" {
		return ""
	}
	return "quarry: warning: " + c.cannotTellSentence("~/Backup/"+recordedID+".sqlite") + "\n"
}

// storeStatus is the Status cell of the recorded snapshot's row, with its leading gap.
func (c recordedClass) storeStatus() string {
	if c.marked {
		return "  store"
	}
	return ""
}

// jsonWarnings is the warnings array snapshots --json prints for c, the path in its absolute form.
func (c recordedClass) jsonWarnings(recorded string) []string {
	if c.reason == "" {
		return []string{}
	}
	return []string{c.cannotTellSentence(recorded)}
}

// snapshotsRecordedCell runs the snapshots command through run beside olderPair and a store built from
// recordedID recorded as c has it; it returns the recorded path and the run's result.
func snapshotsRecordedCell(t *testing.T, c recordedClass, run func(*testing.T) (int, string, string)) (string, int, string, string) {
	t.Helper()
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	recorded := buildStoreFromClass(t, home, recordedID, c)
	exitCode, stdout, stderr := run(t)
	return recorded, exitCode, stdout, stderr
}

func Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	buildStoreFromUnreadable(t, home, recordedID)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, cannotTellPrefix+"cannot read ~/Backup/"+recordedID+".sqlite: permission denied\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}

func Test_run_snapshots_recorded_snapshot_cells(t *testing.T) {
	for _, c := range recordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			_, exitCode, stdout, stderr := snapshotsRecordedCell(t, c, runSnapshots)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stderrWarning(), stderr)
			assert.Equal(t, ""+
				"ID                Taken                   Size  Source        Status\n"+
				"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken"+c.storeStatus()+"\n"+
				"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
				"Total                                   3.5 MB\n", stdout)
		})
	}
}

func Test_run_snapshots_json_recorded_snapshot_cells(t *testing.T) {
	for _, c := range recordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			recorded, exitCode, stdout, stderr := snapshotsRecordedCell(t, c, runSnapshotsJSON)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stderrWarning(), stderr)
			var doc struct {
				StoreSnapshot struct {
					ID   string `json:"id"`
					Path string `json:"path"`
				} `json:"store_snapshot"`
				Snapshots []struct {
					ID    string `json:"id"`
					Store bool   `json:"store"`
				} `json:"snapshots"`
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			assert.Equal(t, recordedID, doc.StoreSnapshot.ID)
			assert.Equal(t, recorded, doc.StoreSnapshot.Path)
			assert.Equal(t, c.marked, doc.Snapshots[0].Store)
			assert.False(t, doc.Snapshots[1].Store)
			assert.Equal(t, c.jsonWarnings(recorded), doc.Warnings)
		})
	}
}
