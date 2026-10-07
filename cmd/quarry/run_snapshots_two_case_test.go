// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
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

// runSnapshotsWithReadDir runs the snapshots command args over a Server that lists its folders through readDir, plus opts.
func runSnapshotsWithReadDir(t *testing.T, home string, readDir func(string) ([]fs.DirEntry, error), opts []snapshot.Option, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(append([]snapshot.Option{
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithReadDir(readDir),
		}, opts...)...), nil
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

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, nil)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
	assert.Equal(t, "quarry: warning: "+snapshotsShown+" holds both "+oldestID+".sqlite and "+oldestID+".SQLITE; "+
		"quarry lists, prunes and uses only "+oldestID+".sqlite; rename or remove the other\n", stderr)
}

func Test_run_snapshots_json_warns_about_a_stray_letter_case_with_the_absolute_folder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, nil, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Snapshots []struct {
			ID string `json:"id"`
		} `json:"snapshots"`
		TotalBytes int64    `json:"total_bytes"`
		Warnings   []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Snapshots, 1)
	assert.Equal(t, oldestID, doc.Snapshots[0].ID)
	assert.Equal(t, int64(oldestBytes), doc.TotalBytes)
	assert.Equal(t, []string{dir + " holds both " + oldestID + ".sqlite and " + oldestID + ".SQLITE; " +
		"quarry lists, prunes and uses only " + oldestID + ".sqlite; rename or remove the other"}, doc.Warnings)
}

func Test_run_snapshots_json_puts_the_stray_letter_case_warning_between_the_config_and_store_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")
	stray := " holds both " + oldestID + ".sqlite and " + oldestID + ".SQLITE; " +
		"quarry lists, prunes and uses only " + oldestID + ".sqlite; rename or remove the other"
	storeWarning := "cannot tell which snapshot the store was built from: the file is not a DuckDB database"

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, []snapshot.Option{
		snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
	}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		dir + stray,
		storeWarning,
	}, doc.Warnings)
	assert.Equal(t, ""+
		"quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		"quarry: warning: "+snapshotsShown+stray+"\n"+
		"quarry: warning: "+storeWarning+"\n", stderr)
}

// recordingRemove records the base name of every file prune asks to remove and removes nothing.
func recordingRemove(removed *[]string) snapshot.Option {
	return snapshot.WithRemove(func(path string) error {
		*removed = append(*removed, filepath.Base(path))
		return nil
	})
}

func Test_run_snapshots_prune_is_silent_about_a_stray_letter_case(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantStdout  func(dir string) string
		wantRemoved []string
	}{
		{
			name: "text",
			args: []string{"prune", "--keep", "1"},
			wantStdout: func(string) string {
				return "Deleted 1 snapshot (1.2 MB), keeping the newest one:\n" +
					"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n"
			},
			wantRemoved: []string{oldestID + ".sqlite"},
		},
		{
			name: "json",
			args: []string{"prune", "--keep", "1", "--json"},
			wantStdout: func(dir string) string {
				return "{\n" +
					"  \"dry_run\": false,\n" +
					"  \"keep\": 1,\n" +
					pruneStoreSnapshotJSON(dir, recordedID) +
					"  \"deleted\": [\n" +
					pruneEntryJSON(dir, oldestID, oldestBytes) + "\n" +
					"  ],\n" +
					"  \"would_delete\": [],\n" +
					"  \"failed\": [],\n" +
					"  \"warnings\": []\n" +
					"}\n"
			},
			wantRemoved: []string{oldestID + ".sqlite"},
		},
		{
			name: "dry run json",
			args: []string{"prune", "--keep", "1", "--dry-run", "--json"},
			wantStdout: func(dir string) string {
				return "{\n" +
					"  \"dry_run\": true,\n" +
					"  \"keep\": 1,\n" +
					pruneStoreSnapshotJSON(dir, recordedID) +
					"  \"deleted\": [],\n" +
					"  \"would_delete\": [\n" +
					pruneEntryJSON(dir, oldestID, oldestBytes) + "\n" +
					"  ],\n" +
					"  \"failed\": [],\n" +
					"  \"warnings\": []\n" +
					"}\n"
			},
			wantRemoved: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, olderPair()...)
			buildStoreFrom(t, home, filepath.Join(dir, recordedID+".sqlite"))
			readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")
			var removed []string

			exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, []snapshot.Option{
				snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
				recordingRemove(&removed),
			}, c.args...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, c.wantStdout(dir), stdout)
			assert.Equal(t, c.wantRemoved, removed)
		})
	}
}
