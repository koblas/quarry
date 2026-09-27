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

func Test_DiscoverBundle_refuses_when_no_bundle_is_found(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, documents string)
	}{
		{name: "Documents is absent", setup: func(t *testing.T, documents string) {}},
		{
			name: "Documents is empty",
			setup: func(t *testing.T, documents string) {
				require.NoError(t, os.MkdirAll(documents, 0o700))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			c.setup(t, filepath.Join(home, "Documents"))

			_, err := snapshot.DiscoverBundle(home)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, "no .quicken file found in ~/Documents; pass one with --quicken <path>", re.Error())
		})
	}
}

func Test_DiscoverBundle_ignores_dotfiles_and_non_directories_and_returns_the_only_bundle(t *testing.T) {
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
	home := t.TempDir()
	bundleDir := filepath.Join(home, "Documents", "Home.QUICKEN")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("x"), 0o600))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, bundleDir, got)
}

func Test_DiscoverBundle_follows_a_symlinked_bundle_and_skips_a_dangling_one(t *testing.T) {
	home := t.TempDir()
	documents := filepath.Join(home, "Documents")
	bundle := v9fixture.OpenBundle(t, documents)
	require.NoError(t, os.Symlink(
		filepath.Join(documents, "does-not-exist"), filepath.Join(documents, "Broken.quicken")))

	got, err := snapshot.DiscoverBundle(home)

	require.NoError(t, err)
	assert.Equal(t, bundle.Dir, got)
}

func Test_DiscoverBundle_refuses_when_multiple_bundles_exist(t *testing.T) {
	cases := []struct {
		name       string
		bundles    []string
		wantStderr string
	}{
		{
			name:       "two arbitrary bundles",
			bundles:    []string{"B.quicken", "A.quicken"},
			wantStderr: "found 2 .quicken files in ~/Documents (A.quicken, B.quicken); choose one with --quicken <path>",
		},
		{
			name:    "the spec's three names created out of order",
			bundles: []string{"Old.quicken", "Business.quicken", "Home.quicken"},
			wantStderr: "found 3 .quicken files in ~/Documents " +
				"(Business.quicken, Home.quicken, Old.quicken); choose one with --quicken <path>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

func Test_DiscoverBundle_refuses_when_the_only_match_has_no_data_file(t *testing.T) {
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
