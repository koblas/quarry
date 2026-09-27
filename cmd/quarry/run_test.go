// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		"Schema", "matches reference hardkoded/quicken-skills@752107b (82 tables, 1,835 columns)",
	)
	assert.Equal(t, want, stdout.String())
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

func Test_run_reports_exit_1_when_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	// frob is an unknown command: if the home-directory guard were bypassed,
	// this would reach cli.Execute and exit 2 instead of 1.
	exitCode := run(context.Background(), []string{"frob"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.NotEmpty(t, stderr.String())
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
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.True(t, strings.HasPrefix(lines[0], "quarry: "))
	assert.Contains(t, lines[0], "build reference schema", "expected the failure to come from v9.Reference, not from a later stage that also observes the cancelled context")
}

// The bundle passes ResolveBundlePath's R4-R7 checks (a real directory with a
// readable regular "data" file) so this exercises srv.Sync's own error path,
// not path resolution.
func Test_run_reports_exit_1_when_sync_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundleDir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.NotEmpty(t, stderr.String())
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
