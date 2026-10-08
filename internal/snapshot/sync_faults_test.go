package snapshot_test

import (
	"context"
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
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sync_fails_when_no_reference_is_configured(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(snapshot.WithSnapshotDir(t.TempDir()))

	_, err := srv.Sync(t.Context(), t.TempDir())

	require.Error(t, err)
}

// Write-safety guard: nothing reaches the snapshots directory before the
// probe succeeds.
func Test_sync_creates_nothing_when_the_probe_fails(t *testing.T) {
	t.Parallel()
	bundleDir := filepath.Join(t.TempDir(), "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
	)

	_, err := srv.Sync(t.Context(), bundleDir)

	require.Error(t, err)
	_, statErr := os.Stat(snapshotsDir)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_sync_wraps_an_error_when_opening_the_bundle_fails(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(t.TempDir()),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{openErr: errBoom}),
	)
	bundlePath := t.TempDir()

	_, err := srv.Sync(t.Context(), bundlePath)

	require.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "sync "+bundlePath)
}

func Test_sync_wraps_an_error_when_the_probe_fails(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(t.TempDir()),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{probeErr: errBoom}),
	)
	bundlePath := t.TempDir()

	_, err := srv.Sync(t.Context(), bundlePath)

	require.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "sync "+bundlePath)
}

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

			assert.Equal(t, c.want, refusalText(t, err))
			assertNoPartialsLeftBehind(t, snapshotsDir)
		})
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

	assert.Equal(t, "Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment", refusalText(t, err))
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

	assert.Equal(t, "Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment", refusalText(t, err))
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

			assert.Equal(t, c.want, refusalText(t, err))
		})
	}
}

const (
	diskFullRefusal = "cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again"
	// A write failure that is neither a permission error nor disk-full/over-quota
	// gets its own text, distinct from the disk-full wording.
	writeFailedRefusal = "cannot write snapshot to ~/snapshots: input/output error; run quarry sync again"
)

// A Discard failure is best-effort: it never replaces the write refusal already classified for the
// write failure that triggered it, and failDiscard only overrides the returned error, so each call
// still removes its own real file.
func Test_sync_refuses_when_writing_to_the_snapshots_folder_fails(t *testing.T) {
	t.Parallel()
	enospc := &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.ENOSPC}
	commitManifest := &os.LinkError{Op: "link", Old: "manifest.json.partial", New: "manifest.json", Err: syscall.ENOSPC}
	commitSnapshot := &os.LinkError{Op: "link", Old: "snapshot.sqlite.partial", New: "snapshot.sqlite", Err: syscall.ENOSPC}
	cases := []struct {
		name                                              string
		failWriteManifest, failCommitManifest, failCommit error
		failDiscard                                       error
		want                                              string
	}{
		{
			name:              "writing the manifest fails with an unclassified cause",
			failWriteManifest: &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.EIO},
			want:              writeFailedRefusal,
		},
		{name: "writing the manifest fails", failWriteManifest: enospc, want: diskFullRefusal},
		{name: "committing the manifest fails", failCommitManifest: commitManifest, want: diskFullRefusal},
		// The manifest final is already committed when CommitSnapshot fails; the
		// empty directory afterward proves Discard removed that final too.
		{name: "committing the snapshot fails", failCommit: commitSnapshot, want: diskFullRefusal},
		{name: "write manifest fails and so does discard", failWriteManifest: enospc, failDiscard: errBoom, want: diskFullRefusal},
		{name: "commit manifest fails and so does discard", failCommitManifest: enospc, failDiscard: errBoom, want: diskFullRefusal},
		{name: "commit snapshot fails and so does discard", failCommit: enospc, failDiscard: errBoom, want: diskFullRefusal},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			snapshotsDir := filepath.Join(home, "snapshots")
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			srv := newDestinationServer(t, home, snapshotsDir, &partialFaultDestination{
				real:               snapshot.NewDirDestination(snapshotsDir),
				failWriteManifest:  c.failWriteManifest,
				failCommitManifest: c.failCommitManifest,
				failCommit:         c.failCommit,
				failDiscard:        c.failDiscard,
			})

			_, err := srv.Sync(t.Context(), bundle.Dir)

			assert.Equal(t, c.want, refusalText(t, err))
			assertSnapshotsDirEmpty(t, snapshotsDir)
		})
	}
}

// mkdirAllCause reproduces os.MkdirAll's own failure against an existing
// blockedPath through a connection independent of the code under test, so a
// test can pin the exact OS reason rather than asserting only "some error".
func mkdirAllCause(t *testing.T, blockedPath string) string {
	t.Helper()
	err := os.MkdirAll(blockedPath, 0o700)
	require.Error(t, err)
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
	return pathErr.Err.Error()
}

// Exercises the real Destination adapter's Prepare fault path: MkdirAll
// fails because the configured snapshots path is already a regular file.
func Test_sync_refuses_when_the_snapshots_directory_cannot_be_prepared(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref := v9Reference(t)
	home := t.TempDir()
	blockedPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))
	cause := mkdirAllCause(t, blockedPath)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(blockedPath),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
	)

	_, err := srv.Sync(t.Context(), bundle.Dir)

	assert.Equal(t,
		"cannot write to "+blockedPath+": "+cause+"; make the directory writable by your user", refusalText(t, err))
}

// partialFaultDestination wraps the real production Destination and injects
// exactly one failing method, so the rest of Sync's pipeline (a real
// backup and schema read) runs for real.
type partialFaultDestination struct {
	real                                              snapshot.Destination
	failWriteManifest, failCommitManifest, failCommit error
	failDiscard                                       error

	backedUpPartial, discardedPartial string
}

func (f *partialFaultDestination) Prepare(ctx context.Context) error { return f.real.Prepare(ctx) }
func (f *partialFaultDestination) Backup(ctx context.Context, src snapshot.Source, name string) (string, string, error) {
	partial, resolvedName, err := f.real.Backup(ctx, src, name)
	f.backedUpPartial = partial
	return partial, resolvedName, err
}

func (f *partialFaultDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	if f.failWriteManifest != nil {
		return "", f.failWriteManifest
	}
	return f.real.WriteManifest(ctx, name, data)
}

func (f *partialFaultDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	if f.failCommitManifest != nil {
		return "", f.failCommitManifest
	}
	return f.real.CommitManifest(ctx, partial)
}

func (f *partialFaultDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	if f.failCommit != nil {
		return "", f.failCommit
	}
	return f.real.CommitSnapshot(ctx, partial)
}

func (f *partialFaultDestination) FinalPaths(name string) (string, string) {
	return f.real.FinalPaths(name)
}

func (f *partialFaultDestination) Discard(ctx context.Context, partial string) error {
	f.discardedPartial = partial
	err := f.real.Discard(ctx, partial)
	if f.failDiscard != nil {
		return f.failDiscard
	}
	return err
}

func Test_sync_still_returns_the_classified_refusal_when_discard_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
	ref := v9Reference(t)
	dest := &partialFaultDestination{
		real:        snapshot.NewDirDestination(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")),
		failDiscard: errBoom,
	}
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(dest),
		snapshot.WithHome(home),
	)

	_, err := srv.Sync(t.Context(), bundle.Dir)

	assert.Equal(t,
		"~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>", refusalText(t, err))
	assert.NotEmpty(t, dest.backedUpPartial)
	assert.Equal(t, dest.backedUpPartial, dest.discardedPartial)
}

// garbageFileOpenCause opens path — garbage bytes buildManifest's own open
// will fail against — through a connection independent of the code under
// test, so a test can pin the exact cause Sync's wrap carries rather than
// asserting only that "some error" occurred.
func garbageFileOpenCause(t *testing.T, path string) error {
	t.Helper()
	conn, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var count int
	return conn.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master").Scan(&count)
}

// An unclassified buildManifest failure names the bundle and carries the
// driver's own cause text, distinct from the integrity-check refusal.
func Test_sync_refuses_a_snapshot_copy_that_cannot_be_opened(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "garbage.sqlite")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))
	cause := garbageFileOpenCause(t, path)
	require.Error(t, cause)

	err := syncSnapshotCopy(t, path)

	assert.Equal(t,
		"cannot read the snapshot of ~/Documents/Home.quicken: "+cause.Error()+"; nothing was kept; run quarry sync again", refusalText(t, err))
}

// A catalog row for a virtual table whose module is absent makes the
// snapshot's own schema read fail; Sync refuses naming the bundle.
func Test_sync_refuses_a_snapshot_whose_schema_cannot_be_read(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "snap.sqlite")
	sqliteFileWith(t, path,
		"CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)",
		"INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')",
		"PRAGMA writable_schema = ON",
		"INSERT INTO sqlite_master (type, name, tbl_name, rootpage, sql) VALUES "+
			"('table', 'ZFOO', 'ZFOO', 0, 'CREATE VIRTUAL TABLE ZFOO USING nonexistent_module')")

	err := syncSnapshotCopy(t, path)

	text := refusalText(t, err)
	assert.True(t, strings.HasPrefix(text, "cannot read the snapshot of ~/Documents/Home.quicken: "))
	assert.Contains(t, text, "no such module")
	assert.True(t, strings.HasSuffix(text, "; nothing was kept; run quarry sync again"))
}

// lastIntegrityCheckLine reads PRAGMA integrity_check's first row's last
// physical line through a connection independent of the code under test.
func lastIntegrityCheckLine(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&row))
	lines := strings.Split(row, "\n")
	return lines[len(lines)-1]
}

func Test_sync_refuses_a_snapshot_that_fails_integrity_check(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "snap.sqlite")
	v9fixture.CorruptDataFile(t, path)

	err := syncSnapshotCopy(t, path)

	assert.Equal(t,
		"the snapshot of ~/Documents/Home.quicken failed SQLite's integrity check ("+lastIntegrityCheckLine(t, path)+
			"); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again", refusalText(t, err))
}

func Test_sync_refuses_a_snapshot_with_no_accounts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
		want string
	}{
		{
			name: "no accounts table",
			ddl:  "CREATE TABLE OTHER (id INTEGER PRIMARY KEY)",
			want: "~/Documents/Home.quicken is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>",
		},
		{
			name: "no account rows",
			ddl:  "CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)",
			want: "~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "snapshot.sqlite")
			sqliteFileWith(t, path, c.ddl)

			err := syncSnapshotCopy(t, path)

			assert.Equal(t, c.want, refusalText(t, err))
		})
	}
}
