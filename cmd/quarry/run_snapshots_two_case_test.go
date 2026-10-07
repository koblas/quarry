// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renamedEntry reports name in place of the name of the real entry it wraps, keeping that entry's type and Info.
type renamedEntry struct {
	fs.DirEntry

	name string
}

func (e renamedEntry) Name() string { return e.name }

// readDirWithVariant is os.ReadDir plus one more regular entry named variant that wraps the real entry named like.
// A macOS volume cannot hold two letter cases of one name, so the second case exists only in this listing.
func readDirWithVariant(variant, like string) func(string) ([]fs.DirEntry, error) {
	return func(dir string) ([]fs.DirEntry, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Name() == like {
				entries = append(entries, renamedEntry{DirEntry: entry, name: variant})
			}
		}
		return entries, nil
	}
}

// runSnapshotsWithReadDir runs the snapshots command args over a Server that lists its folders through readDir.
func runSnapshotsWithReadDir(t *testing.T, home string, readDir func(string) ([]fs.DirEntry, error), args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithReadDir(readDir),
		), nil
	}
	exitCode := runWith(context.Background(), append([]string{"snapshots"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

func Test_run_snapshots_lists_one_of_two_letter_cases_and_warns_naming_both(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home,
		snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true})
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
	assert.Equal(t, "quarry: warning: "+snapshotsShown+" holds both "+oldestID+".sqlite and "+oldestID+".SQLITE; "+
		"quarry lists, prunes and uses only "+oldestID+".sqlite; rename or remove the other\n", stderr)
}
