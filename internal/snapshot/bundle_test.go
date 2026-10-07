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
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "Documents", "Missing.quicken")

	_, err := snapshot.ResolveBundlePath(home, path)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Missing.quicken does not exist; check the path passed to --quicken", re.Error())
}

func Test_ResolveBundlePath_refuses_when_the_top_level_path_cannot_be_statted(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	_, err = conn.ExecContext(t.Context(), "PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, statErr := os.Stat(path + "-wal")
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_ResolveBundlePath_accepts_a_symlinked_data_file(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

const (
	notABundleTail = " is not a Quicken for Mac file (expected a .quicken bundle containing a data file); "
	configPath     = "~/Library/Application Support/quarry/config.toml"
)

func Test_ResolveBundle_prefers_the_flag_path_over_the_configured_one(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	flagged := v9fixture.OpenBundle(t, filepath.Join(home, "Flagged"))
	configured := v9fixture.OpenBundle(t, filepath.Join(home, "Configured"))

	got, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Flag: flagged.Dir, Configured: configured.Dir})

	require.NoError(t, err)
	assert.Equal(t, flagged.Dir, got)
}

func Test_ResolveBundle_uses_the_configured_path_when_no_flag_path_is_given(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "A.quicken"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "B.quicken"), 0o700))

	got, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Configured: "~/Books/Home.quicken"})

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_ResolveBundle_discovers_the_sole_bundle_when_neither_path_is_given(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	got, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{})

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_ResolveBundle_refuses_a_bad_configured_path(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, home string)
		path  string
		want  string
	}{
		{
			name:  "missing",
			setup: func(_ *testing.T, _ string) {},
			path:  "~/Books/Missing.quicken",
			want:  "~/Books/Missing.quicken does not exist; check quicken.path in " + configPath + ", or pass the file with --quicken <path>",
		},
		{
			name: "a plain file",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Books"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(home, "Books", "notes.txt"), []byte("x"), 0o600))
			},
			path: "~/Books/notes.txt",
			want: "~/Books/notes.txt" + notABundleTail + "set quicken.path in " + configPath + " to the .quicken bundle",
		},
		{
			name: "a bundle without a data file",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Books", "Empty.quicken"), 0o700))
			},
			path: "~/Books/Empty.quicken",
			want: "~/Books/Empty.quicken" + notABundleTail + "set quicken.path in " + configPath + " to the .quicken bundle",
		},
		{
			name: "a Quicken for Windows file",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Books"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(home, "Books", "Home.qdf"), []byte("x"), 0o600))
			},
			path: "~/Books/Home.qdf",
			want: "~/Books/Home.qdf is a Quicken for Windows file; quarry reads only Quicken Classic for Mac .quicken files",
		},
		{
			name: "a bundle not open in Quicken",
			setup: func(t *testing.T, home string) {
				t.Helper()
				v9fixture.ClosedWALBundle(t, filepath.Join(home, "Books"))
			},
			path: "~/Books/Home.quicken",
			want: "~/Books/Home.quicken is not open in Quicken (its database has no write-ahead log); " +
				"open it in Quicken, then run quarry sync again",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			c.setup(t, home)

			_, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Configured: c.path})

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, c.want, re.Error())
		})
	}
}

func Test_ResolveBundle_refuses_a_configured_bundle_whose_data_is_unreadable(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
	require.NoError(t, os.Chmod(bundle.DataPath, 0o000))

	_, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Configured: "~/Books/Home.quicken"})

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "cannot read ~/Books/Home.quicken/data: permission denied; "+
		"allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions",
		re.Error())
}

func Test_ResolveBundle_keeps_the_flag_refusals_when_quicken_path_is_also_set(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, home string)
		flag  string
		want  string
	}{
		{
			name:  "missing",
			setup: func(_ *testing.T, _ string) {},
			flag:  "~/Other/Missing.quicken",
			want:  "~/Other/Missing.quicken does not exist; check the path passed to --quicken",
		},
		{
			name: "a plain file",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Other"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(home, "Other", "notes.txt"), []byte("x"), 0o600))
			},
			flag: "~/Other/notes.txt",
			want: "~/Other/notes.txt" + notABundleTail + "pass the .quicken bundle with --quicken <path>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
			c.setup(t, home)

			_, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Flag: c.flag, Configured: "~/Books/Home.quicken"})

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, c.want, re.Error())
		})
	}
}

func Test_ResolveBundle_accepts_a_configured_path_with_a_trailing_slash_or_through_a_symlink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, home string)
		path  string
		want  string
	}{
		{
			name:  "trailing slash",
			setup: func(_ *testing.T, _ string) {},
			path:  "~/Books/Home.quicken/",
			want:  "Books/Home.quicken",
		},
		{
			name: "symlink to the bundle",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.Symlink(filepath.Join(home, "Books", "Home.quicken"), filepath.Join(home, "Link.quicken")))
			},
			path: "~/Link.quicken",
			want: "Link.quicken",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
			c.setup(t, home)

			got, err := snapshot.ResolveBundle(home, snapshot.BundleChoice{Configured: c.path})

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(home, c.want), got)
		})
	}
}
