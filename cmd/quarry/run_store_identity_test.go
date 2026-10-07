// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// betweenLinkID is a valid snapshot ID newer than pruneOldest and older than every other fixture.
	betweenLinkID = "20260928T000000Z"
	// futureLinkID is a valid snapshot ID newer than any a sync mints and older than newerSnapshots'.
	futureLinkID = "29980101T000000Z"
	// absentID is a valid snapshot ID no fixture carries.
	absentID = "20260801T120000Z"
)

// snapshotFile is the path of id's snapshot in dir.
func snapshotFile(dir, id string) string { return filepath.Join(dir, id+".sqlite") }

// hardLink makes link a second name for the file at target and returns link.
func hardLink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Link(target, link))
	return link
}

// symlink makes link a symbolic link to target and returns link.
func symlink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Symlink(target, link))
	return link
}

// fileNames returns the name of every file in dir, sorted.
func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

// symlinkWithManifest symlinks home's latest.sqlite to id's snapshot in dir, spelling its extension as ext, puts
// its manifest beside the link as latest.json so sync --from accepts it, and returns the link.
func symlinkWithManifest(t *testing.T, home, dir, id, ext string) string {
	t.Helper()
	hardLink(t, filepath.Join(dir, id+".json"), filepath.Join(home, "latest.json"))
	return symlink(t, filepath.Join(dir, id+ext), filepath.Join(home, "latest.sqlite"))
}

func Test_run_snapshots_prune_run_twice_never_deletes_the_snapshot_the_recorded_path_resolves_to(t *testing.T) {
	recorded := []string{pruneOldest + ".json", pruneOldest + ".sqlite"}
	newest := []string{pruneNewest + ".json", pruneNewest + ".sqlite"}
	recordedAndNewest := slices.Concat(recorded, newest)
	withNewerLink := slices.Concat(recorded, []string{betweenLinkID + ".sqlite"}, newest)
	withOlderLink := slices.Concat([]string{linkID + ".sqlite"}, recorded, newest)
	cases := []struct {
		name string
		// recorded arranges links around the five snapshots in dir and returns the path the store recorded.
		recorded func(t *testing.T, home, dir string) string
		left     []string
	}{
		{
			name:     "a snapshot in the folder",
			recorded: func(_ *testing.T, _, dir string) string { return snapshotFile(dir, pruneOldest) },
			left:     recordedAndNewest,
		},
		{
			name: "a snapshot in the folder named in another letter case, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return snapshotFile(dir, strings.ToLower(pruneOldest))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink outside the folder to a snapshot named with an upper-case extension, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, filepath.Join(dir, pruneOldest+".SQLITE"), filepath.Join(home, "latest.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink outside the folder whose target has a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, snapshotFile(dir, pruneOldest), filepath.Join(home, "latest.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink in the folder under an older id whose target has a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
			},
			left: slices.Concat([]string{linkID + ".sqlite"}, withNewerLink),
		},
		{
			name: "a snapshot in the folder with a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return snapshotFile(dir, pruneOldest)
			},
			left: withNewerLink,
		},
		{
			name: "a snapshot in the folder with an older hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
				return snapshotFile(dir, pruneOldest)
			},
			left: withOlderLink,
		},
		{
			name: "a hard link outside the folder under no snapshot's id keeps every snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return hardLink(t, snapshotFile(dir, pruneOldest), filepath.Join(home, "linked.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a path that is gone whose id is a snapshot's",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), pruneOldest)
			},
			left: recordedAndNewest,
		},
		{
			name: "a path that is gone whose id is no snapshot's keeps only the newest",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), absentID)
			},
			left: []string{pruneNewest + ".json", pruneNewest + ".sqlite"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, c.recorded(t, home, dir))

			firstExit, _, firstStderr := runPrune(t, "--keep", "1")
			secondExit, _, secondStderr := runPrune(t, "--keep", "1")

			require.Equal(t, 0, firstExit, firstStderr)
			require.Equal(t, 0, secondExit, secondStderr)
			assert.Equal(t, c.left, fileNames(t, dir))
		})
	}
}

// storeBuiltFromASymlink syncs, rebuilds the store --from a symlink to that snapshot spelled with ext and adds a newer
// hard link of it beside one newer snapshot; it returns the snapshot's ID, the newer snapshot's ID and the folder.
func storeBuiltFromASymlink(t *testing.T, home, ext string) (string, string, string) {
	t.Helper()
	newer := newerSnapshots(1)
	id, dir := syncThenWrite(t, home, newer...)
	exitCode, _, stderr := runSyncFrom(t, symlinkWithManifest(t, home, dir, id, ext))
	require.Equal(t, 0, exitCode, stderr)
	hardLink(t, snapshotFile(dir, id), snapshotFile(dir, futureLinkID))
	return id, newer[0].id, dir
}

func Test_run_snapshots_json_marks_the_target_of_the_symlink_sync_built_the_store_from(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _, _ := storeBuiltFromASymlink(t, home, ".sqlite")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, _ := storeIdentity(t, stdout)
	assert.Equal(t, []string{id}, marked)
}

func Test_run_snapshots_json_marks_the_target_of_a_symlink_that_spells_its_extension_in_upper_case(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skipOnCaseSensitiveVolume(t, home)
	id, _, _ := storeBuiltFromASymlink(t, home, ".SQLITE")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, _ := storeIdentity(t, stdout)
	assert.Equal(t, []string{id}, marked)
}

func Test_run_snapshots_prune_run_twice_keeps_the_target_of_the_symlink_sync_built_the_store_from(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, newerID, dir := storeBuiltFromASymlink(t, home, ".sqlite")

	firstExit, _, firstStderr := runPrune(t, "--keep", "1")
	secondExit, _, secondStderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, firstExit, firstStderr)
	require.Equal(t, 0, secondExit, secondStderr)
	assert.Equal(t,
		[]string{id + ".json", id + ".sqlite", futureLinkID + ".sqlite", newerID + ".json", newerID + ".sqlite"}, fileNames(t, dir))
}

func Test_run_sync_from_keeps_the_snapshot_it_names_and_its_newer_hard_link_beyond_the_newest_12(t *testing.T) {
	cases := []struct {
		name string
		// from arranges a way to name id's snapshot in dir and returns the --from value.
		from func(t *testing.T, home, dir, id string) string
	}{
		{
			name: "a symlink to the snapshot",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				return symlinkWithManifest(t, home, dir, id, ".sqlite")
			},
		},
		{
			name: "a symlink to the snapshot named with an upper-case extension",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				return symlinkWithManifest(t, home, dir, id, ".SQLITE")
			},
		},
		{
			name: "the snapshot's path with an upper-case extension",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				return filepath.Join(dir, id+".SQLITE")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			older := oldSnapshots(1)
			id, dir := syncThenWrite(t, home, append(newerSnapshots(keptSnapshots), older...)...)
			from := c.from(t, home, dir, id)
			hardLink(t, snapshotFile(dir, id), snapshotFile(dir, futureLinkID))

			exitCode, stdout, stderr := runSyncFrom(t, from)

			require.Equal(t, 0, exitCode, stderr)
			assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 and the store's own \(`, stdout)
			requireSnapshotsKept(t, dir, id)
			assert.FileExists(t, snapshotFile(dir, futureLinkID))
			assert.NoFileExists(t, snapshotFile(dir, older[0].id))
		})
	}
}

func Test_run_snapshots_prune_keeps_the_snapshot_sync_was_built_from_under_another_extension_once_that_file_is_gone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	newer := newerSnapshots(1)
	id, dir := syncThenWrite(t, home, newer...)
	backup := filepath.Join(home, "backup")
	require.NoError(t, os.MkdirAll(backup, 0o700))
	from := hardLink(t, snapshotFile(dir, id), filepath.Join(backup, id+".db"))
	hardLink(t, filepath.Join(dir, id+".json"), from+".json")
	exitCode, _, stderr := runSyncFrom(t, from)
	require.Equal(t, 0, exitCode, stderr)
	require.NoError(t, os.Remove(from))
	require.NoError(t, os.Remove(from+".json"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Nothing to delete: 2 snapshots, within the newest one and "+id+", the store's snapshot\n", stdout)
	requireSnapshotsKept(t, dir, id, newer[0].id)
}
