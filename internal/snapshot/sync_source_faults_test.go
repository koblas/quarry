package snapshot_test

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Backup failure classifies as a destination write fault (disk-full,
// quota, permission), or an unclassified source-side read fault otherwise.
func Test_sync_refuses_when_the_backup_fails(t *testing.T) {
	t.Parallel()
	fullCause := sqlite3.Error{Code: sqlite3.ErrFull}.Error()
	quotaCause := syscall.EDQUOT.Error()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an unclassified cause",
			err:  errBoom,
			want: "cannot copy ~/Documents/Home.quicken: boom; run quarry sync again",
		},
		{
			name: "disk full reported through a PathError",
			err:  &fs.PathError{Op: "write", Path: "partial", Err: syscall.ENOSPC},
			want: "cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		},
		{
			name: "over quota reported through a PathError",
			err:  &fs.PathError{Op: "write", Path: "partial", Err: syscall.EDQUOT},
			want: "cannot write snapshot to ~/snapshots: " + quotaCause + "; free disk space, then run quarry sync again",
		},
		{
			// The real sqliteSource.Backup wraps SQLite's own SQLITE_FULL, which carries no errno.
			name: "disk full reported by SQLite itself, with no errno",
			err: fmt.Errorf("backup: %w", fmt.Errorf("backup to %s: %w",
				"partial", sqlite3.Error{Code: sqlite3.ErrFull})),
			want: "cannot write snapshot to ~/snapshots: " + fullCause + "; free disk space, then run quarry sync again",
		},
		{
			name: "a permission error",
			err:  &fs.PathError{Op: "write", Path: "partial", Err: fs.ErrPermission},
			want: "cannot write to ~/snapshots: permission denied; make the directory writable by your user",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			snapshotsDir := filepath.Join(home, "snapshots")
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(snapshotsDir),
				snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
				snapshot.WithSource(&fakeSource{backupErr: c.err}),
				snapshot.WithHome(home),
			)

			_, err := srv.Sync(t.Context(), filepath.Join(home, "Documents", "Home.quicken"))

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, c.want, re.Error())
			assertNoPartialsLeftBehind(t, snapshotsDir)
		})
	}
}

// assertNoPartialsLeftBehind fails if dir holds any of Destination's
// exclusively-created ".partial" files: every Destination method that
// creates one removes it again on its own failure.
func assertNoPartialsLeftBehind(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasSuffix(entry.Name(), ".partial"), "leftover partial: %s", entry.Name())
	}
}

// A Backup error carrying both a classified sqlite cause and fs.ErrPermission
// must still resolve to the busy source refusal.
func Test_sync_reports_the_source_refusal_when_a_backup_error_is_also_a_permission_error(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	combined := fmt.Errorf("%w: %w", sqlite3.Error{Code: sqlite3.ErrBusy}, fs.ErrPermission)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(t.TempDir()),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{backupErr: combined}),
		snapshot.WithHome(home),
	)

	_, err := srv.Sync(t.Context(), filepath.Join(home, "Documents", "Home.quicken"))

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment", re.Error())
}

// Uses a rollback-journal (non-WAL) fixture: v9fixture's WAL-mode bundle
// does not block a mode=ro reader against an uncommitted writer.
func Test_sync_refuses_a_busy_bundle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")
	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(t.Context(), "CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	require.NoError(t, err)

	locker, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	locker.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = locker.Close() })
	lockerConn, err := locker.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = lockerConn.Close() })
	_, err = lockerConn.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	_, err = lockerConn.ExecContext(t.Context(), "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Locked')")
	require.NoError(t, err)

	snapshotsDir := filepath.Join(home, "snapshots")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithHome(home),
		snapshot.WithBusyTimeout(80*time.Millisecond),
	)

	_, err = srv.Sync(t.Context(), bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment", re.Error())
	_, statErr := os.Stat(snapshotsDir)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// Every Source call (Open, Probe, Backup) shares the same busy/encrypted
// classification, independent of real lock or file-content timing.
func Test_sync_refuses_when_the_source_reports_a_classified_error(t *testing.T) {
	t.Parallel()
	const busyMsg = "Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment"
	const notADBMsg = "~/Documents/Home.quicken is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again"

	cases := []struct {
		name string
		fake *fakeSource
		want string
	}{
		{name: "open busy", fake: &fakeSource{openErr: sqlite3.Error{Code: sqlite3.ErrBusy}}, want: busyMsg},
		{name: "open encrypted", fake: &fakeSource{openErr: sqlite3.Error{Code: sqlite3.ErrNotADB}}, want: notADBMsg},
		{name: "probe busy", fake: &fakeSource{probeErr: sqlite3.Error{Code: sqlite3.ErrBusy}}, want: busyMsg},
		{name: "probe encrypted", fake: &fakeSource{probeErr: sqlite3.Error{Code: sqlite3.ErrNotADB}}, want: notADBMsg},
		{name: "backup busy", fake: &fakeSource{backupErr: sqlite3.Error{Code: sqlite3.ErrBusy}}, want: busyMsg},
		{name: "backup encrypted", fake: &fakeSource{backupErr: sqlite3.Error{Code: sqlite3.ErrNotADB}}, want: notADBMsg},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(t.TempDir()),
				snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
				snapshot.WithSource(c.fake),
				snapshot.WithHome(home),
			)

			_, err := srv.Sync(t.Context(), filepath.Join(home, "Documents", "Home.quicken"))

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, c.want, re.Error())
		})
	}
}
