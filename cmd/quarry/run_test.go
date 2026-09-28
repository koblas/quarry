// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter fails every Write with err, so a test can prove what happens
// when stdout itself cannot be written to (a full disk on the far end of a
// pipe, for example).
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func Test_run_writes_a_verified_snapshot_and_reports_success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 2 accounts\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
	)
	assert.Equal(t, want, stdout.String())
}

// The leftovers are backdated past the sweep's age gate: a fresh leftover
// could belong to another sync still in flight.
func Test_run_removes_leftover_partials_silently_before_syncing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
	leftover := filepath.Join(snapshotsDir, ".20260101T000000Z.sqlite.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("crash debris"), 0o600))
	leftoverWAL := leftover + "-wal"
	require.NoError(t, os.WriteFile(leftoverWAL, []byte("wal"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(leftover, old, old))
	require.NoError(t, os.Chtimes(leftoverWAL, old, old))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Snapshot  ")
	assert.NotContains(t, stdout.String(), ".partial")
	_, err := os.Stat(leftover)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(leftoverWAL)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// A stdout write failure still leaves the already-committed snapshot and
// manifest on disk, named in the refusal rather than the write error alone.
func Test_run_reports_exit_1_when_writing_stdout_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	writeErr := errors.New("no space left on device")
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; the snapshot is kept at "+
			abbreviated(t, snapshotPath, home)+" and its .json manifest holds the full result\n",
		stderr.String())
}

func Test_run_prints_the_manifest_as_json_with_the_json_flag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	want, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), stdout.String())

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.ElementsMatch(t, []string{"snapshot", "schema", "warnings"}, slices.Collect(maps.Keys(parsed)))
}

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

func Test_run_sync_help_names_both_documents_folders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Without --quicken, quarry looks for .quicken files in ~/Documents and in\n"+
		"~/Library/Application Support/Quicken/Documents, and uses the one it finds\n"+
		"if there is exactly one.")
	assert.Contains(t, stdout.String(),
		"path to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)")
}

func Test_run_refuses_an_empty_or_whitespace_quicken_flag_as_a_usage_error_even_with_a_bundle_in_documents(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{"sync", "--quicken="}},
		{name: "all whitespace", args: []string{"sync", "--quicken=   "}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n", stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func Test_run_rejects_usage_errors(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "positional argument",
			args:       []string{"sync", "~/x.quicken"},
			wantStderr: "quarry: sync takes no arguments; pass the file with --quicken <path>\n",
		},
		{
			name:       "unknown flag",
			args:       []string{"sync", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "flag missing its value",
			args:       []string{"sync", "--quicken"},
			wantStderr: "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "unknown command",
			args:       []string{"frob"},
			wantStderr: "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "near miss of a known command",
			args:       []string{"synk"},
			wantStderr: "quarry: unknown command \"synk\" for \"quarry\"; Run 'quarry sync --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			require.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			assert.NotContains(t, stderr.String(), "Did you mean")
		})
	}
}

// Home resolution happens only inside sync's RunE, on a valid sync
// invocation with HOME unset.
func Test_run_reports_exit_1_when_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again\n",
		stderr.String())
}

func Test_run_help_and_usage_errors_do_not_need_home(t *testing.T) {
	t.Setenv("HOME", "")

	t.Run("root help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("sync help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("unknown command is still a usage error, not the home-directory refusal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"frob"}, &stdout, &stderr)

		assert.Equal(t, 2, exitCode)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry sync --help' for usage.\n", stderr.String())
	})
}

func Test_run_reports_exit_1_when_the_context_is_already_cancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer

	exitCode := run(ctx, []string{"sync", "--quicken", filepath.Join(home, "Any.quicken")}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: sync interrupted; nothing was kept; run quarry sync again\n", stderr.String())
}

// The bundle itself is valid; its data file is present but not a SQLite
// database at all, so this exercises srv.Sync's error path, not path resolution.
func Test_run_refuses_an_encrypted_bundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundleDir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundleDir, home)+
		" is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again\n",
		stderr.String())
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// A WAL-formatted bundle with no live -wal file must be refused before Sync
// opens it — that open alone would create -wal/-shm this test checks for.
func Test_run_refuses_a_bundle_that_is_not_open_in_quicken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.ClosedWALBundle(t, filepath.Join(home, "Documents"))
	before, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundle.Dir, home)+
		" is not open in Quicken (its database has no write-ahead log); open it in Quicken, then run quarry sync again\n",
		stderr.String())
	after, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)
	assert.Equal(t, entryNames(before), entryNames(after))
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// entryNames returns entries' names in order.
func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// Prepare's MkdirAll is a no-op on an already-existing directory regardless
// of its permission bits, so the snapshots directory must exist before the
// chmod, or the failure this test wants would never surface.
func Test_run_refuses_a_snapshots_directory_that_is_not_writable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
	t.Cleanup(func() { _ = os.Chmod(snapshotsDir, 0o700) })
	require.NoError(t, os.Chmod(snapshotsDir, 0o500))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot write to "+abbreviated(t, snapshotsDir, home)+
		": permission denied; make the directory writable by your user\n",
		stderr.String())
	entries, err := os.ReadDir(snapshotsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// The snapshots directory is left behind, empty, in every case: Prepare
// already ran before the content check can fail.
func Test_run_refuses_a_bundle_whose_snapshot_content_is_rejected(t *testing.T) {
	cases := []struct {
		name        string
		buildBundle func(t *testing.T, home string) string
		wantLine    func(t *testing.T, bundleDir, home string) string
	}{
		{
			name: "damaged so only integrity_check fails",
			buildBundle: func(t *testing.T, home string) string {
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				v9fixture.CorruptDataFile(t, filepath.Join(bundleDir, "data"))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				last := integrityCheckLastLine(t, filepath.Join(bundleDir, "data"))
				return "quarry: the snapshot of " + abbreviated(t, bundleDir, home) +
					" failed SQLite's integrity check (" + last +
					"); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again"
			},
		},
		{
			name: "missing ZACCOUNT (including a 0-byte data file)",
			buildBundle: func(t *testing.T, home string) string {
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), nil, 0o600))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>"
			},
		},
		{
			name: "ZACCOUNT with no rows",
			buildBundle: func(t *testing.T, home string) string {
				bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
				return bundle.Dir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bundleDir := c.buildBundle(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync", "--quicken", bundleDir}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
			require.Len(t, lines, 1)
			assert.Equal(t, c.wantLine(t, bundleDir, home), lines[0])
			snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
			entries, err := os.ReadDir(snapshotsDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// The reference count line and rows never change with the schema outcome:
// only the Schema line and its rows differ.
func Test_run_reports_a_schema_mismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"  - table   %s\n  - column  %s.%s\n  - column  %s.%s\n  + column  %s.%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "DIFFERS from reference hardkoded/quicken-skills@752107b+quarry.1: "+
			"1 table and 2 columns missing, 1 column not in reference",
		v9fixture.MissingSchemaDroppedTable,
		v9fixture.MissingSchemaDroppedColumnTable, v9fixture.MissingSchemaDroppedColumn1,
		v9fixture.MissingSchemaDroppedColumnTable, v9fixture.MissingSchemaDroppedColumn2,
		v9fixture.MissingSchemaAddedColumnTable, v9fixture.MissingSchemaAddedColumn,
	)
	assert.Equal(t, want, stdout.String())

	assert.Equal(t, "quarry: schema check failed: Home.quicken is missing 1 table and 2 columns "+
		"that the schema reference expects; the snapshot is kept at "+abbreviated(t, snapshotPath, home)+
		" and the diff is in its .json manifest; quarry cannot import this file until its schema reference is updated\n",
		stderr.String())

	rawManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	assert.Equal(t, []any{}, manifest["warnings"])
	schema, ok := manifest["schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, schema["verified"])
}

func Test_run_reports_extra_schema_only_as_a_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.ExtraSchemaBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"  + table   %s\n  + column  %s.%s\n  + column  %s.%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns), "+
			"plus 1 table and 2 columns not in it",
		v9fixture.ExtraSchemaAddedTable,
		v9fixture.ExtraSchemaAddedColumnTable1, v9fixture.ExtraSchemaAddedColumn1,
		v9fixture.ExtraSchemaAddedColumnTable2, v9fixture.ExtraSchemaAddedColumn2,
	)
	assert.Equal(t, want, stdout.String())

	wantWarning := "quarry: warning: Home.quicken has 1 table and 2 columns that are not in the schema reference; " +
		"quarry ignores them (listed in " + filepath.Base(manifestPath) + ")\n"
	assert.Equal(t, wantWarning, stderr.String())

	rawManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	assert.Equal(t, []any{strings.TrimSuffix(strings.TrimPrefix(wantWarning, "quarry: warning: "), "\n")},
		manifest["warnings"])
}

func Test_run_reports_a_schema_mismatch_as_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	want, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), stdout.String())

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	var schema map[string]any
	require.NoError(t, json.Unmarshal(parsed["schema"], &schema))
	assert.Equal(t, false, schema["verified"])
	assert.NotEmpty(t, schema["missing_tables"])
	assert.NotEmpty(t, schema["missing_columns"])
	assert.NotEmpty(t, schema["unexpected_columns"])

	require.NotEmpty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: schema check failed:"))
}

// integrityCheckLastLine reads PRAGMA integrity_check's first row's last
// physical line through a connection independent of the code under test.
func integrityCheckLastLine(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&row))
	lines := strings.Split(row, "\n")
	return lines[len(lines)-1]
}

// onlyFileWithSuffix fails the test unless exactly one entry in dir ends in
// suffix, returning its full path.
func onlyFileWithSuffix(t *testing.T, dir, suffix string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = append(found, e.Name())
		}
	}
	require.Len(t, found, 1, "expected exactly one %s file in %s, found %v", suffix, dir, found)
	return filepath.Join(dir, found[0])
}

// abbreviated mirrors the CLI's ~-abbreviation so the expected string is
// built from the real path, not a re-derived one.
func abbreviated(t *testing.T, path, home string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(path, home+string(filepath.Separator)))
	return "~" + strings.TrimPrefix(path, home)
}

// megabytes mirrors the CLI's decimal-MB rounding for a fixture small enough
// that thousands-grouping never applies.
func megabytes(bytes int64) string {
	tenths := (bytes*10 + 500000) / 1000000
	return fmt.Sprintf("%d.%d MB", tenths/10, tenths%10)
}
