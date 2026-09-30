package snapshot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
