// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each path-fault row's setup leaves no manifest file at all: that absence
// proves the path pre-check runs before any manifest read is attempted.
func Test_run_refuses_from_input_that_is_not_a_usable_snapshot(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (home, from string)
		want  func(t *testing.T, home, from string) string
	}{
		{
			name: "a path that does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				return home, filepath.Join(snapshotsDir, "20260927T143005Z.sqlite")
			},
			want: func(t *testing.T, home, from string) string {
				t.Helper()
				return "quarry: " + abbreviated(t, from, home) + " does not exist; check the path passed to --from"
			},
		},
		{
			name: "a directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				dir := filepath.Join(snapshotsDir, "20260927T143005Z.sqlite")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				return home, dir
			},
			want: func(t *testing.T, home, from string) string {
				t.Helper()
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				return "quarry: " + abbreviated(t, from, home) + " is not a snapshot file; pass a .sqlite snapshot from " +
					abbreviated(t, snapshotsDir, home) + " with --from <snapshot>"
			},
		},
		{
			name: "a .quicken bundle",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				bundle := v9fixture.OpenBundle(t, t.TempDir())
				return home, bundle.Dir
			},
			want: func(t *testing.T, _, from string) string {
				t.Helper()
				return "quarry: " + from + " is a Quicken file, not a snapshot; " +
					"pass it with --quicken <path>, or pass a snapshot with --from <snapshot>"
			},
		},
		{
			name: "a file with no manifest",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(snapshotsDir, "20260927T143005Z.sqlite"), []byte("x"), 0o600))
				return home, "20260927T143005Z"
			},
			want: func(t *testing.T, home, _ string) string {
				t.Helper()
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				path := filepath.Join(snapshotsDir, "20260927T143005Z.sqlite")
				return "quarry: " + abbreviated(t, path, home) +
					" is not a quarry snapshot (no .json manifest next to it); pass a snapshot taken by quarry sync with --from <snapshot>"
			},
		},
		{
			name: "an unreadable file",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				if os.Geteuid() == 0 {
					t.Skip("root ignores file permissions")
				}
				home := t.TempDir()
				t.Setenv("HOME", home)
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
				path := filepath.Join(snapshotsDir, "20260927T143005Z.sqlite")
				content := []byte("x")
				require.NoError(t, os.WriteFile(path, content, 0o600))
				writeManifestForTest(t, path, content)
				require.NoError(t, os.Chmod(path, 0o000))
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
				return home, "20260927T143005Z"
			},
			want: func(t *testing.T, home, _ string) string {
				t.Helper()
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				path := filepath.Join(snapshotsDir, "20260927T143005Z.sqlite")
				return "quarry: cannot read " + abbreviated(t, path, home) + ": permission denied; check the file's permissions"
			},
		},
		{
			name: "a snapshot whose hash changed",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
				var stdout, stderr bytes.Buffer
				require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr))
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
				manifestPath := filepath.Join(snapshotsDir, id+".json")
				raw, err := os.ReadFile(manifestPath)
				require.NoError(t, err)
				var manifest snapshot.Manifest
				require.NoError(t, json.Unmarshal(raw, &manifest))
				manifest.Snapshot.SHA256 = strings.Repeat("0", 64)
				data, err := manifest.Encode()
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(manifestPath, data, 0o600))
				require.NoError(t, os.Remove(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")))
				return home, id
			},
			want: func(t *testing.T, home, from string) string {
				t.Helper()
				snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
				path := filepath.Join(snapshotsDir, from+".sqlite")
				return "quarry: " + abbreviated(t, path, home) +
					" has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, from := c.setup(t)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync", "--from", from}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.want(t, home, from)+"\n", stderr.String())
			_, err := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func Test_run_sync_from_an_unknown_id_points_at_quarry_snapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", "20260601T090000Z"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no snapshot 20260601T090000Z in ~/Library/Application Support/quarry/snapshots; "+
		"run quarry snapshots to list the ones kept\n", stderr.String())
}

// writeManifestForTest writes the .json manifest next to snapshotPath, recording content's real SHA-256.
func writeManifestForTest(t *testing.T, snapshotPath string, content []byte) {
	t.Helper()
	sum := sha256.Sum256(content)
	manifest := snapshot.Manifest{Snapshot: snapshot.Info{
		Source: "/Users/x/Documents/Home.quicken", SHA256: hex.EncodeToString(sum[:]),
	}}
	data, err := manifest.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(strings.TrimSuffix(snapshotPath, ".sqlite")+".json", data, 0o600))
}
