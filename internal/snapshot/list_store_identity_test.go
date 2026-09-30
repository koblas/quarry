package snapshot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Snapshot IDs given to links: newer than every snapshot threeSnapshots writes, between its middle
// and newest, and older than all of them.
const (
	idLinkNewer   = "20261001T000000Z"
	idLinkBetween = "20260930T000000Z"
	idLinkOlder   = "20260901T000000Z"
)

// skipOnCaseSensitiveVolume skips t unless the volume under dir resolves a file name in any letter case.
func skipOnCaseSensitiveVolume(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "caseprobe"), nil, 0o600))
	if _, err := os.Stat(filepath.Join(dir, "CASEPROBE")); err != nil {
		t.Skip("the volume is case-sensitive: a differently-cased name does not resolve")
	}
}

// snapshotFile is the path of id's snapshot in dir.
func snapshotFile(dir, id string) string { return filepath.Join(dir, id+".sqlite") }

// hardLink makes link a second name for the file at target and returns link.
func hardLink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Link(target, link))
	return link
}

// symlink makes link a symbolic link to target, creating link's folder, and returns link.
func symlink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
	require.NoError(t, os.Symlink(target, link))
	return link
}

// markedIDs returns the IDs of the entries l marks as the store's, newest first.
func markedIDs(l snapshot.Listing) []string {
	ids := []string{}
	for _, entry := range l.Entries {
		if entry.Store {
			ids = append(ids, entry.ID)
		}
	}
	return ids
}

func Test_list_marks_exactly_the_snapshot_the_recorded_path_resolves_to(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// recorded arranges links around the three snapshots in dir and returns the path the store recorded.
		recorded func(t *testing.T, home, dir string) string
		want     []string
	}{
		{
			name:     "a snapshot in the folder",
			recorded: func(_ *testing.T, _, dir string) string { return snapshotFile(dir, idMiddle) },
			want:     []string{idMiddle},
		},
		{
			name: "a snapshot in the folder named in another letter case, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return snapshotFile(dir, strings.ToLower(idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink outside the folder whose target has a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "latest.sqlite"))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink in the folder under an older id whose target has a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkOlder))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink outside the folder named as a newer hard link of its target",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), snapshotFile(home, idLinkNewer))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink named as a snapshot to a hard link of it outside the folder under another name",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				outside := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "elsewhere.sqlite"))
				return symlink(t, outside, snapshotFile(filepath.Join(home, "aliases"), idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a snapshot in the folder with a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return snapshotFile(dir, idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a snapshot in the folder with an older hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkOlder))
				return snapshotFile(dir, idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a hard link outside the folder under no snapshot's id marks the newest snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
				return hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))
			},
			want: []string{idLinkBetween},
		},
		{
			name: "a hard link outside the folder under another snapshot's id marks the snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				return hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(home, idNewest))
			},
			want: []string{idMiddle},
		},
		{
			name: "a copy outside the folder under a snapshot's id",
			recorded: func(t *testing.T, home, _ string) string {
				t.Helper()
				return writeSnapshot(t, home, idMiddle, 1000)
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's in another letter case",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), strings.ToLower(idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is no snapshot's marks nothing",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), idOutside)
			},
			want: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := threeSnapshots(t, home)
			recorded := c.recorded(t, home, dir)

			listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, markedIDs(listing))
		})
	}
}
