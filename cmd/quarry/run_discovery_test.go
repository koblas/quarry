// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_refuses_a_bad_quicken_path(t *testing.T) {
	cases := []struct {
		name       string
		quicken    string
		setup      func(t *testing.T, home string)
		wantStderr string
	}{
		{
			name:    "missing",
			quicken: "~/Documents/Missing.quicken",
			setup:   func(t *testing.T, home string) {},
			wantStderr: "quarry: ~/Documents/Missing.quicken does not exist; " +
				"check the path passed to --quicken\n",
		},
		{
			name:    "ending .QDF",
			quicken: "~/Documents/Home.QDF",
			setup: func(t *testing.T, home string) {
				bundle := v9fixture.OpenBundle(t, filepath.Join(home, "RealBundle"))
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents"), 0o700))
				require.NoError(t, os.Symlink(bundle.Dir, filepath.Join(home, "Documents", "Home.QDF")))
			},
			wantStderr: "quarry: ~/Documents/Home.QDF is a Quicken for Windows file; " +
				"quarry reads only Quicken Classic for Mac .quicken files\n",
		},
		{
			name:    "a plain file",
			quicken: "~/Documents/Plain.quicken",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(home, "Documents", "Plain.quicken"), []byte("x"), 0o600))
			},
			wantStderr: "quarry: ~/Documents/Plain.quicken is not a Quicken for Mac file " +
				"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>\n",
		},
		{
			name:    "a bundle without data",
			quicken: "~/Documents/Empty.quicken",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "Empty.quicken"), 0o700))
			},
			wantStderr: "quarry: ~/Documents/Empty.quicken is not a Quicken for Mac file " +
				"(expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>\n",
		},
		{
			name:    "a bundle whose data is unreadable",
			quicken: "~/Documents/Home.quicken",
			setup: func(t *testing.T, home string) {
				if os.Geteuid() == 0 {
					t.Skip("root ignores file permissions")
				}
				bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
				require.NoError(t, os.Chmod(bundle.DataPath, 0o000))
			},
			wantStderr: "quarry: cannot read ~/Documents/Home.quicken/data: permission denied; " +
				"allow your terminal to access the folder in System Settings > Privacy & Security, " +
				"or check the file's permissions\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync", "--quicken", c.quicken}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}

	t.Run("--quicken names one of two bundles, so discovery never runs", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
		require.NoError(t, os.MkdirAll(filepath.Join(quickenDocumentsDir(home), "Other.quicken"), 0o700))
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

		require.Equal(t, 0, exitCode)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, bundle.Dir, home)+"\n")
	})

	t.Run("a valid bundle given as ~/…", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync", "--quicken", "~/Documents/Home.quicken"}, &stdout, &stderr)

		require.Equal(t, 0, exitCode)
		assert.Empty(t, stderr.String())
		assert.NotEmpty(t, stdout.String())
	})
}

func Test_run_discovers_the_bundle_from_documents_without_quicken(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, home string) string // returns the discovered bundle's dir
	}{
		{
			name: "exactly one bundle in ~/Documents",
			setup: func(t *testing.T, home string) string {
				return v9fixture.OpenBundle(t, filepath.Join(home, "Documents")).Dir
			},
		},
		{
			name: "~/Documents missing, Quicken Documents folder has one",
			setup: func(t *testing.T, home string) string {
				return v9fixture.OpenBundle(t, quickenDocumentsDir(home)).Dir
			},
		},
		{
			name: "an ancestor of the Quicken Documents folder is a regular file, ~/Documents has one",
			setup: func(t *testing.T, home string) string {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Dir(quickenDocumentsDir(home))), 0o700))
				require.NoError(t, os.WriteFile(filepath.Dir(quickenDocumentsDir(home)), []byte("x"), 0o600))
				return v9fixture.OpenBundle(t, filepath.Join(home, "Documents")).Dir
			},
		},
		{
			name: "the Quicken Documents folder itself is a regular file, ~/Documents has one",
			setup: func(t *testing.T, home string) string {
				require.NoError(t, os.MkdirAll(filepath.Dir(quickenDocumentsDir(home)), 0o700))
				require.NoError(t, os.WriteFile(quickenDocumentsDir(home), []byte("x"), 0o600))
				return v9fixture.OpenBundle(t, filepath.Join(home, "Documents")).Dir
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bundleDir := c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

			require.Equal(t, 0, exitCode)
			assert.Empty(t, stderr.String())
			assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, bundleDir, home)+"\n")
		})
	}
}

// quickenDocumentsDir is Quicken Classic for Mac's own Documents folder
// under home.
func quickenDocumentsDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Quicken", "Documents")
}

func Test_run_pools_bundles_across_both_documents_folders(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, home string)
		wantStderr string
	}{
		{
			name:  "both folders missing",
			setup: func(t *testing.T, home string) {},
			wantStderr: "quarry: no .quicken file found in ~/Documents or " +
				"~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>\n",
		},
		{
			name: "both folders exist and are empty",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents"), 0o700))
				require.NoError(t, os.MkdirAll(quickenDocumentsDir(home), 0o700))
			},
			wantStderr: "quarry: no .quicken file found in ~/Documents or " +
				"~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>\n",
		},
		{
			name: "bundles only under the Quicken Backups folder",
			setup: func(t *testing.T, home string) {
				backups := filepath.Join(home, "Library", "Application Support", "Quicken", "Backups")
				require.NoError(t, os.MkdirAll(filepath.Join(backups, "Home.quicken"), 0o700))
			},
			wantStderr: "quarry: no .quicken file found in ~/Documents or " +
				"~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>\n",
		},
		{
			name: "one bundle in each folder",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "A.quicken"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(quickenDocumentsDir(home), "B.quicken"), 0o700))
			},
			wantStderr: "quarry: found 2 .quicken files (~/Documents/A.quicken, " +
				"~/Library/Application Support/Quicken/Documents/B.quicken); choose one with --quicken <path>\n",
		},
		{
			name: "same basename in both folders, three distinct bundles",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "Business.quicken"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "Home.quicken"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(quickenDocumentsDir(home), "Home.quicken"), 0o700))
			},
			wantStderr: "quarry: found 3 .quicken files (~/Documents/Business.quicken, ~/Documents/Home.quicken, " +
				"~/Library/Application Support/Quicken/Documents/Home.quicken); choose one with --quicken <path>\n",
		},
		{
			name: "two bundles in ~/Documents, none in the Quicken folder",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "A.quicken"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "B.quicken"), 0o700))
			},
			wantStderr: "quarry: found 2 .quicken files (~/Documents/A.quicken, ~/Documents/B.quicken); " +
				"choose one with --quicken <path>\n",
		},
		{
			name: "two bundles in the Quicken folder, none in ~/Documents",
			setup: func(t *testing.T, home string) {
				require.NoError(t, os.MkdirAll(filepath.Join(quickenDocumentsDir(home), "A.quicken"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(quickenDocumentsDir(home), "B.quicken"), 0o700))
			},
			wantStderr: "quarry: found 2 .quicken files (~/Library/Application Support/Quicken/Documents/A.quicken, " +
				"~/Library/Application Support/Quicken/Documents/B.quicken); choose one with --quicken <path>\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func Test_run_counts_a_bundle_reached_two_ways_once(t *testing.T) {
	t.Run("~/Documents/Linked.quicken symlinks to the only bundle in the Quicken folder", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		bundle := v9fixture.OpenBundle(t, quickenDocumentsDir(home))
		require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents"), 0o700))
		link := filepath.Join(home, "Documents", "Linked.quicken")
		require.NoError(t, os.Symlink(bundle.Dir, link))
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

		require.Equal(t, 0, exitCode)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, link, home)+"\n")
	})

	t.Run("the Quicken folder is symlinked to ~/Documents", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
		require.NoError(t, os.MkdirAll(filepath.Dir(quickenDocumentsDir(home)), 0o700))
		require.NoError(t, os.Symlink(filepath.Join(home, "Documents"), quickenDocumentsDir(home)))
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

		require.Equal(t, 0, exitCode)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, bundle.Dir, home)+"\n")
	})
}

func Test_run_refuses_when_a_discovery_location_is_unreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	cases := []struct {
		name       string
		setup      func(t *testing.T, home string)
		wantStderr string
	}{
		{
			name: "~/Documents unreadable, Quicken folder has one",
			setup: func(t *testing.T, home string) {
				v9fixture.OpenBundle(t, quickenDocumentsDir(home))
				documents := filepath.Join(home, "Documents")
				require.NoError(t, os.MkdirAll(documents, 0o700))
				t.Cleanup(func() { _ = os.Chmod(documents, 0o700) })
				require.NoError(t, os.Chmod(documents, 0o000))
			},
			wantStderr: "quarry: cannot read ~/Documents: permission denied; allow your terminal to access " +
				"the Documents folder in System Settings > Privacy & Security > Files and Folders, " +
				"or pass --quicken <path>\n",
		},
		{
			name: "~/Documents has one, Quicken folder unreadable",
			setup: func(t *testing.T, home string) {
				v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
				quickenDir := quickenDocumentsDir(home)
				require.NoError(t, os.MkdirAll(quickenDir, 0o700))
				t.Cleanup(func() { _ = os.Chmod(quickenDir, 0o700) })
				require.NoError(t, os.Chmod(quickenDir, 0o000))
			},
			wantStderr: "quarry: cannot read ~/Library/Application Support/Quicken/Documents: permission denied; " +
				"check the folder's permissions, or pass --quicken <path>\n",
		},
		{
			name: "both unreadable",
			setup: func(t *testing.T, home string) {
				documents := filepath.Join(home, "Documents")
				require.NoError(t, os.MkdirAll(documents, 0o700))
				t.Cleanup(func() { _ = os.Chmod(documents, 0o700) })
				require.NoError(t, os.Chmod(documents, 0o000))
				quickenDir := quickenDocumentsDir(home)
				require.NoError(t, os.MkdirAll(quickenDir, 0o700))
				t.Cleanup(func() { _ = os.Chmod(quickenDir, 0o700) })
				require.NoError(t, os.Chmod(quickenDir, 0o000))
			},
			wantStderr: "quarry: cannot read ~/Documents: permission denied; allow your terminal to access " +
				"the Documents folder in System Settings > Privacy & Security > Files and Folders, " +
				"or pass --quicken <path>\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}
