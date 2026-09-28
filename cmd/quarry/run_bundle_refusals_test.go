// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bundle itself is valid; its data file is present but not a SQLite
// database at all, so this exercises srv.Sync's error path, not path resolution.
func Test_run_refuses_an_encrypted_bundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundleDir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundleDir, home)+
		" is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again\n",
		stderr.String())
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// A WAL-formatted bundle with no live -wal file must be refused before Sync
// opens it — that open alone would create -wal/-shm this test checks for.
func Test_run_refuses_a_bundle_that_is_not_open_in_quicken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.ClosedWALBundle(t, filepath.Join(home, "Documents"))
	before, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundle.Dir, home)+
		" is not open in Quicken (its database has no write-ahead log); open it in Quicken, then run quarry sync again\n",
		stderr.String())
	after, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)
	assert.Equal(t, entryNames(before), entryNames(after))
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// entryNames returns entries' names in order.
func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// Prepare's MkdirAll is a no-op on an already-existing directory regardless
// of its permission bits, so the snapshots directory must exist before the
// chmod, or the failure this test wants would never surface.
func Test_run_refuses_a_snapshots_directory_that_is_not_writable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
	t.Cleanup(func() { _ = os.Chmod(snapshotsDir, 0o700) })
	require.NoError(t, os.Chmod(snapshotsDir, 0o500))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot write to "+abbreviated(t, snapshotsDir, home)+
		": permission denied; make the directory writable by your user\n",
		stderr.String())
	entries, err := os.ReadDir(snapshotsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// The snapshots directory is left behind, empty, in every case: Prepare
// already ran before the content check can fail.
func Test_run_refuses_a_bundle_whose_snapshot_content_is_rejected(t *testing.T) {
	cases := []struct {
		name        string
		buildBundle func(t *testing.T, home string) string
		wantLine    func(t *testing.T, bundleDir, home string) string
	}{
		{
			name: "damaged so only integrity_check fails",
			buildBundle: func(t *testing.T, home string) string {
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				v9fixture.CorruptDataFile(t, filepath.Join(bundleDir, "data"))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				last := integrityCheckLastLine(t, filepath.Join(bundleDir, "data"))
				return "quarry: the snapshot of " + abbreviated(t, bundleDir, home) +
					" failed SQLite's integrity check (" + last +
					"); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again"
			},
		},
		{
			name: "missing ZACCOUNT (including a 0-byte data file)",
			buildBundle: func(t *testing.T, home string) string {
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), nil, 0o600))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>"
			},
		},
		{
			name: "ZACCOUNT with no rows",
			buildBundle: func(t *testing.T, home string) string {
				bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
				return bundle.Dir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bundleDir := c.buildBundle(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync", "--quicken", bundleDir}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
			require.Len(t, lines, 1)
			assert.Equal(t, c.wantLine(t, bundleDir, home), lines[0])
			snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
			entries, err := os.ReadDir(snapshotsDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// integrityCheckLastLine reads PRAGMA integrity_check's first row's last
// physical line through a connection independent of the code under test.
func integrityCheckLastLine(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&row))
	lines := strings.Split(row, "\n")
	return lines[len(lines)-1]
}
