package snapshot_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource is a hand-written Source fake: each call returns the
// configured error, or succeeds when it is nil.
type fakeSource struct {
	openErr, probeErr, backupErr error
}

func (f *fakeSource) Open(context.Context, string) error   { return f.openErr }
func (f *fakeSource) Probe(context.Context) error          { return f.probeErr }
func (f *fakeSource) Backup(context.Context, string) error { return f.backupErr }
func (f *fakeSource) Close() error                         { return nil }

var errBoom = errors.New("boom")

func Test_sync_fails_when_no_reference_is_configured(t *testing.T) {
	srv := snapshot.NewServer(snapshot.WithSnapshotDir(t.TempDir()))

	_, err := srv.Sync(t.Context(), t.TempDir())

	require.Error(t, err)
}

// Mandatory write-safety guard (BR-8): nothing reaches the snapshots
// directory before the probe succeeds.
func Test_sync_creates_nothing_when_the_probe_fails(t *testing.T) {
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

// A generic backup failure is not a classified source error (busy/encrypted),
// so it falls to R14, not the old generic "sync <path>:" wrap.
func Test_sync_wraps_an_error_when_the_backup_fails(t *testing.T) {
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{backupErr: errBoom}),
		snapshot.WithHome(home),
	)
	bundlePath := t.TempDir()

	_, err := srv.Sync(t.Context(), bundlePath)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: boom; free disk space, then run quarry sync again",
		re.Error())
	assertNoPartialsLeftBehind(t, snapshotsDir)
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
// Prepare's failure is always R13, whatever the OS reason.
func Test_sync_wraps_an_error_when_the_snapshots_directory_cannot_be_prepared(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	home := t.TempDir()
	blockedPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))
	cause := mkdirAllCause(t, blockedPath)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(blockedPath),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write to "+blockedPath+": "+cause+"; make the directory writable by your user",
		re.Error())
}

func Test_sync_wraps_an_error_when_the_probe_fails(t *testing.T) {
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

// fixedPathDestination hands Backup's caller a pre-built file instead of
// really backing anything up, so a test controls exactly what buildManifest
// reads.
type fixedPathDestination struct {
	snapshotPath string
}

func (f *fixedPathDestination) Prepare(context.Context) error { return nil }
func (f *fixedPathDestination) Backup(_ context.Context, _ snapshot.Source, name string) (string, string, error) {
	return f.snapshotPath, name, nil
}
func (f *fixedPathDestination) WriteManifest(context.Context, string, []byte) (string, error) {
	return "", nil
}
func (f *fixedPathDestination) CommitSnapshot(context.Context, string) (string, error) {
	return "", nil
}
func (f *fixedPathDestination) CommitManifest(context.Context, string) (string, error) {
	return "", nil
}
func (f *fixedPathDestination) FinalPaths(string) (string, string) { return "", "" }
func (f *fixedPathDestination) Discard(context.Context, string) error { return nil }

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
	return conn.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&count)
}

func Test_sync_wraps_an_error_when_the_snapshot_cannot_be_opened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.sqlite")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))
	cause := garbageFileOpenCause(t, path)
	require.Error(t, cause)
	bundlePath := t.TempDir()
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: path}),
	)

	_, err := srv.Sync(t.Context(), bundlePath)

	require.ErrorIs(t, err, cause)
	assert.ErrorContains(t, err, "sync "+bundlePath)
	var re snapshot.RefusalError
	assert.False(t, errors.As(err, &re), "an unrelated open failure must not be misclassified as a content refusal")
}

// lastIntegrityCheckLine reads PRAGMA integrity_check's first row's last
// physical line through a connection independent of the code under test.
func lastIntegrityCheckLine(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&row))
	lines := strings.Split(row, "\n")
	return lines[len(lines)-1]
}

func Test_sync_refuses_a_snapshot_that_fails_integrity_check(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	path := filepath.Join(t.TempDir(), "snap.sqlite")
	v9fixture.CorruptDataFile(t, path)
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: path}),
		snapshot.WithHome(home),
	)

	_, err := srv.Sync(t.Context(), bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"the snapshot of ~/Documents/Home.quicken failed SQLite's integrity check ("+lastIntegrityCheckLine(t, path)+
			"); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again",
		re.Error())
}

func Test_sync_refuses_a_snapshot_with_no_accounts_table(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-accounts.sqlite")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE OTHER (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: path}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"~/Documents/Home.quicken is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>",
		re.Error())
}

func Test_sync_refuses_a_snapshot_with_no_account_rows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-accounts.sqlite")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: path}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>",
		re.Error())
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

// assertSnapshotsDirEmpty fails if dir holds anything at all: unlike
// assertNoPartialsLeftBehind, it also catches a committed final Discard
// should have removed.
func assertSnapshotsDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func Test_sync_wraps_an_error_when_writing_the_manifest_fails(t *testing.T) {
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:              snapshot.NewDirDestination(snapshotsDir),
			failWriteManifest: &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

func Test_sync_wraps_an_error_when_committing_the_manifest_fails(t *testing.T) {
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:               snapshot.NewDirDestination(snapshotsDir),
			failCommitManifest: &os.LinkError{Op: "link", Old: "manifest.json.partial", New: "manifest.json", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

// The manifest final is already committed when CommitSnapshot fails; the
// empty directory afterward proves Discard removed that final too, not
// only the still-open snapshot partial.
func Test_sync_wraps_an_error_when_committing_the_snapshot_fails(t *testing.T) {
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:       snapshot.NewDirDestination(snapshotsDir),
			failCommit: &os.LinkError{Op: "link", Old: "snapshot.sqlite.partial", New: "snapshot.sqlite", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

func Test_sync_still_returns_the_classified_refusal_when_discard_fails(t *testing.T) {
	home := t.TempDir()
	bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	dest := &partialFaultDestination{
		real:        snapshot.NewDirDestination(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")),
		failDiscard: errBoom,
	}
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(dest),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>",
		re.Error())
	assert.NotEmpty(t, dest.backedUpPartial)
	assert.Equal(t, dest.backedUpPartial, dest.discardedPartial)
}

// A Backup error carrying both a classified sqlite cause and fs.ErrPermission
// must still resolve to the busy source refusal: sourceRefusal's classified
// check runs before the write-failure classifier ever sees the error.
func Test_sync_reports_the_source_refusal_when_a_backup_error_is_also_a_permission_error(t *testing.T) {
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

// A Discard failure is best-effort (SCENARIO-08's rule): it never replaces
// the R14 refusal already classified for the write failure that triggered it.
func Test_sync_still_returns_the_R14_refusal_when_discard_fails_after_a_write_failure(t *testing.T) {
	writeErr := &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.ENOSPC}

	cases := []struct {
		name               string
		failWriteManifest  error
		failCommitManifest error
		failCommit         error
	}{
		{name: "write manifest fails", failWriteManifest: writeErr},
		{name: "commit manifest fails", failCommitManifest: writeErr},
		{name: "commit snapshot fails", failCommit: writeErr},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			snapshotsDir := filepath.Join(home, "snapshots")
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			ref, err := v9.Reference(t.Context())
			require.NoError(t, err)
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(snapshotsDir),
				snapshot.WithReference(v9.ReferenceLabel, ref),
				snapshot.WithDestination(&partialFaultDestination{
					real:               snapshot.NewDirDestination(snapshotsDir),
					failWriteManifest:  c.failWriteManifest,
					failCommitManifest: c.failCommitManifest,
					failCommit:         c.failCommit,
					failDiscard:        errBoom,
				}),
				snapshot.WithHome(home),
			)

			_, err = srv.Sync(t.Context(), bundle.Dir)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t,
				"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
				re.Error())
			// The fake's failDiscard only overrides the returned error;
			// each Discard call still removes its own real file, so a
			// case with two Discard calls must leave the directory empty
			// even though both calls report failure.
			assertSnapshotsDirEmpty(t, snapshotsDir)
		})
	}
}

// Uses a rollback-journal (non-WAL) fixture: v9fixture's WAL-mode bundle
// does not block a mode=ro reader against an uncommitted writer.
func Test_sync_refuses_a_busy_bundle(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")
	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec("CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
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
	assertNoPartialsLeftBehind(t, snapshotsDir)
	_, statErr := os.Stat(snapshotsDir)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// Proves every Source call Sync routes through sourceRefusal (Open, Probe
// and Backup) shares the same busy/encrypted classification, independent of
// real lock or file-content timing.
func Test_sync_refuses_when_the_source_reports_a_classified_error(t *testing.T) {
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
