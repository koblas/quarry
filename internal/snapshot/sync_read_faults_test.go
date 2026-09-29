package snapshot_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// An unclassified buildManifest failure names the bundle and carries the
// driver's own cause text, distinct from the integrity-check refusal.
func Test_sync_refuses_a_snapshot_copy_that_cannot_be_opened(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "garbage.sqlite")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))
	cause := garbageFileOpenCause(t, path)
	require.Error(t, cause)
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
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
		"cannot read the snapshot of ~/Documents/Home.quicken: "+cause.Error()+"; nothing was kept; run quarry sync again",
		re.Error())
}

// A catalog row for a virtual table whose module is absent makes the
// snapshot's own schema read fail; Sync refuses naming the bundle.
func Test_sync_refuses_a_snapshot_whose_schema_cannot_be_read(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "snap.sqlite")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	require.NoError(t, err)
	_, err = conn.Exec("PRAGMA writable_schema = ON")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO sqlite_master (type, name, tbl_name, rootpage, sql) VALUES " +
		"('table', 'ZFOO', 'ZFOO', 0, 'CREATE VIRTUAL TABLE ZFOO USING nonexistent_module')")
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
	assert.True(t, strings.HasPrefix(re.Error(), "cannot read the snapshot of ~/Documents/Home.quicken: "))
	assert.Contains(t, re.Error(), "no such module")
	assert.True(t, strings.HasSuffix(re.Error(), "; nothing was kept; run quarry sync again"))
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
