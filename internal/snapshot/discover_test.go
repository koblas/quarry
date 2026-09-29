package snapshot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DiscoverBundle_refuses_when_no_bundle_is_found(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, documents string)
	}{
		{name: "Documents is absent", setup: func(_ *testing.T, _ string) {}},
		{
			name: "Documents is empty",
			setup: func(t *testing.T, documents string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(documents, 0o700))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			c.setup(t, filepath.Join(home, "Documents"))

			_, err := snapshot.DiscoverBundle(home)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, "no .quicken file found in ~/Documents or "+
				"~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>", re.Error())
		})
	}
}

func Test_DiscoverBundle_ignores_dotfiles_and_non_directories_and_returns_the_only_bundle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	bundle := v9fixture.OpenBundle(t, documents)
	require.NoError(t, os.MkdirAll(filepath.Join(documents, ".Hidden.quicken"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(documents, "Notes.txt"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(documents, "Plain.quicken"), []byte("x"), 0o600))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_DiscoverBundle_finds_a_bundle_whose_suffix_case_differs(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.QUICKEN")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("x"), 0o600))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, bundleDir, got)
}

func Test_DiscoverBundle_skips_a_dangling_symlink_and_finds_the_real_bundle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	bundle := v9fixture.OpenBundle(t, documents)
	require.NoError(t, os.Symlink(
		filepath.Join(documents, "does-not-exist"), filepath.Join(documents, "Broken.quicken")))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_DiscoverBundle_follows_a_symlink_to_a_bundle_outside_documents(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	require.NoError(t, os.MkdirAll(documents, 0o700))

	elsewhere := t.TempDir()
	realBundle := filepath.Join(elsewhere, "Real.quicken")
	require.NoError(t, os.MkdirAll(realBundle, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(realBundle, "data"), []byte("x"), 0o600))

	link := filepath.Join(documents, "Linked.quicken")
	require.NoError(t, os.Symlink(realBundle, link))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, link, got, "the symlink path itself, not the resolved target, is the discovered bundle")
}

// A symlink loop is a real stat fault, not a missing file: DiscoverBundle
// must not silently fall back to the one bundle it can see.
func Test_DiscoverBundle_refuses_when_a_candidate_cannot_be_statted_for_a_reason_other_than_not_existing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dir  func(home string) string
	}{
		{name: "under ~/Documents", dir: func(home string) string { return filepath.Join(home, "Documents") }},
		{name: "under the Quicken folder", dir: func(home string) string {
			return filepath.Join(home, "Library", "Application Support", "Quicken", "Documents")
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := c.dir(home)
			v9fixture.OpenBundle(t, dir)
			loop := filepath.Join(dir, "Loop.quicken")
			require.NoError(t, os.Symlink(loop, loop))
			_, statErr := os.Stat(loop)
			require.Error(t, statErr)
			var errno syscall.Errno
			require.ErrorAs(t, statErr, &errno, "the loop must fail with a raw errno to assert the cause text against")

			_, err := snapshot.DiscoverBundle(home)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, fmt.Sprintf("cannot read %s: ", homepath.Abbreviate(home, loop))+errno.Error()+
				"; allow your terminal to access the folder in System Settings > Privacy & Security, "+
				"or check the file's permissions", re.Error())
		})
	}
}

func Test_DiscoverBundle_refuses_when_multiple_bundles_exist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		bundles    []string
		wantStderr string
	}{
		{
			name:    "two arbitrary bundles",
			bundles: []string{"B.quicken", "A.quicken"},
			wantStderr: "found 2 .quicken files (~/Documents/A.quicken, ~/Documents/B.quicken); " +
				"choose one with --quicken <path>",
		},
		{
			name:    "three names created out of order",
			bundles: []string{"Old.quicken", "Business.quicken", "Home.quicken"},
			wantStderr: "found 3 .quicken files (~/Documents/Business.quicken, ~/Documents/Home.quicken, " +
				"~/Documents/Old.quicken); choose one with --quicken <path>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			documents := filepath.Join(home, "Documents")
			for _, name := range c.bundles {
				require.NoError(t, os.MkdirAll(filepath.Join(documents, name), 0o700))
			}

			_, err := snapshot.DiscoverBundle(home)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, c.wantStderr, re.Error())
		})
	}
}

func Test_DiscoverBundle_groups_the_bundle_count_by_thousands(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	for i := range 1000 {
		require.NoError(t, os.MkdirAll(filepath.Join(documents, fmt.Sprintf("%04d.quicken", i)), 0o700))
	}

	_, err := snapshot.DiscoverBundle(home)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "found 1,000 .quicken files", strings.SplitN(re.Error(), " (", 2)[0])
}

func Test_DiscoverBundle_refuses_when_the_only_match_has_no_data_file(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "Home.quicken"), 0o700))

	_, err := snapshot.DiscoverBundle(home)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken is not a Quicken for Mac file "+
		"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		re.Error())
}

func Test_DiscoverBundle_refuses_when_documents_is_unreadable(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	require.NoError(t, os.MkdirAll(documents, 0o700))
	t.Cleanup(func() { _ = os.Chmod(documents, 0o700) })
	require.NoError(t, os.Chmod(documents, 0o000))

	_, err := snapshot.DiscoverBundle(home)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Contains(t, re.Error(), "permission denied")
}

// Only the Quicken location treats ENOTDIR as missing, even though a valid bundle sits there.
func Test_DiscoverBundle_refuses_documents_as_unreadable_when_it_is_a_regular_file(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "Documents"), []byte("x"), 0o600))
	v9fixture.OpenBundle(t, filepath.Join(home, "Library", "Application Support", "Quicken", "Documents"))

	_, err := snapshot.DiscoverBundle(home)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "cannot read ~/Documents: "+syscall.ENOTDIR.Error()+"; allow your terminal to access "+
		"the Documents folder in System Settings > Privacy & Security > Files and Folders, "+
		"or pass --quicken <path>", re.Error())
}
