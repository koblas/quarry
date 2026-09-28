package snapshot_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ResolveBundlePath_refuses_a_missing_path(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Documents", "Missing.quicken")

	_, err := snapshot.ResolveBundlePath(home, path)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Missing.quicken does not exist; check the path passed to --quicken", re.Error())
}

func Test_ResolveBundlePath_refuses_when_the_top_level_path_cannot_be_statted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	lockedDir := filepath.Join(home, "Locked")
	require.NoError(t, os.MkdirAll(lockedDir, 0o700))
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o700) })
	require.NoError(t, os.Chmod(lockedDir, 0o000))
	path := filepath.Join(lockedDir, "Home.quicken")

	_, err := snapshot.ResolveBundlePath(home, path)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "cannot read ~/Locked/Home.quicken: permission denied; "+
		"allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions",
		re.Error())
}

func Test_ResolveBundlePath_refuses_a_qdf_suffix(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name string
		ext  string
	}{
		{name: "lowercase", ext: ".qdf"},
		{name: "uppercase", ext: ".QDF"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(home, "Home"+c.ext)
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

			_, err := snapshot.ResolveBundlePath(home, path)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Contains(t, re.Error(), "Quicken for Windows file")
		})
	}
}

func Test_ResolveBundlePath_refuses_a_plain_file(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	_, err := snapshot.ResolveBundlePath(home, path)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken is not a Quicken for Mac file "+
		"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		re.Error())
}

func Test_ResolveBundlePath_refuses_a_bundle_without_data(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))

	_, err := snapshot.ResolveBundlePath(home, bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken is not a Quicken for Mac file "+
		"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		re.Error())
}

func Test_ResolveBundlePath_refuses_a_bundle_whose_data_is_a_directory(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "data"), 0o700))

	_, err := snapshot.ResolveBundlePath(home, bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken is not a Quicken for Mac file "+
		"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		re.Error())
}

func Test_ResolveBundlePath_refuses_unreadable_data(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	require.NoError(t, os.Chmod(bundle.DataPath, 0o000))

	_, err := snapshot.ResolveBundlePath(home, bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Contains(t, re.Error(), "permission denied")
}

func Test_ResolveBundlePath_refuses_an_unreadable_bundle_directory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	t.Cleanup(func() { _ = os.Chmod(bundleDir, 0o700) })
	require.NoError(t, os.Chmod(bundleDir, 0o000))

	_, err := snapshot.ResolveBundlePath(home, bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Contains(t, re.Error(), "permission denied")
}

func Test_ResolveBundlePath_refuses_a_wal_formatted_bundle_with_no_live_wal_file(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")
	closeWALFormattedDatabase(t, dataPath)
	before, err := os.ReadDir(bundleDir)
	require.NoError(t, err)

	_, err = snapshot.ResolveBundlePath(home, bundleDir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken is not open in Quicken (its database has no write-ahead log); "+
		"open it in Quicken, then run quarry sync again", re.Error())
	after, err := os.ReadDir(bundleDir)
	require.NoError(t, err)
	assert.Equal(t, namesOf(before), namesOf(after))
}

// closeWALFormattedDatabase leaves path's header marked WAL (bytes 18-19
// == 2) with no live -wal file: SQLite's automatic checkpoint-on-close
// merges and removes the WAL, but the header's format-version bytes stay
// at 2 once journal_mode has ever been WAL.
func closeWALFormattedDatabase(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	_, err = conn.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, statErr := os.Stat(path + "-wal")
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_ResolveBundlePath_accepts_a_symlinked_data_file(t *testing.T) {
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	realFile := filepath.Join(home, "real-data")
	require.NoError(t, os.WriteFile(realFile, []byte("x"), 0o600))
	require.NoError(t, os.Symlink(realFile, filepath.Join(bundleDir, "data")))

	got, err := snapshot.ResolveBundlePath(home, bundleDir)

	require.NoError(t, err)
	assert.Equal(t, bundleDir, got)
}

func Test_ResolveBundlePath_resolves_a_relative_path_to_absolute(t *testing.T) {
	home := t.TempDir()
	v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	t.Chdir(filepath.Join(home, "Documents"))
	wantDir, err := os.Getwd()
	require.NoError(t, err)

	got, err := snapshot.ResolveBundlePath(home, "Home.quicken")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wantDir, "Home.quicken"), got)
}

func Test_ResolveBundlePath_expands_a_tilde_path_under_home(t *testing.T) {
	home := t.TempDir()
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	got, err := snapshot.ResolveBundlePath(home, "~/Documents/Home.quicken")

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_ResolveBundlePath_returns_an_error_when_the_working_directory_no_longer_exists(t *testing.T) {
	home := t.TempDir()
	deletedDir := filepath.Join(t.TempDir(), "deleted")
	require.NoError(t, os.Mkdir(deletedDir, 0o700))
	t.Chdir(deletedDir)
	require.NoError(t, os.Remove(deletedDir))
	if _, err := os.Getwd(); err == nil {
		t.Skip("os.Getwd resolved despite the working directory being removed on this platform")
	}

	_, err := snapshot.ResolveBundlePath(home, "Home.quicken")

	require.Error(t, err)
}
