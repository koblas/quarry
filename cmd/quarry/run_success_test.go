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

	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 2 accounts\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", abbreviated(t, storePath, home),
		"Rows", "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags",
		"Balances", "no accounts to check; 2 never reconciled",
		"Splits", "no transactions to check",
		"Transfers", "none",
		"Findings", "none open",
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
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(leftoverWAL)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// Once the build was reached, a stdout write failure points at --from
// --json instead of the manifest: the store result no longer lives there alone.
func Test_run_points_to_from_when_writing_stdout_fails_after_the_build(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	writeErr := errNoSpace
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; run quarry sync --from "+
			snapshotID(snapshotPath)+" --json to see it again\n",
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
	manifestBytes, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.ElementsMatch(t, []string{"snapshot", "schema", "store", "pruned", "warnings"}, slices.Collect(maps.Keys(parsed)))
	assert.JSONEq(t, string(manifest["snapshot"]), string(parsed["snapshot"]))
	assert.JSONEq(t, string(manifest["schema"]), string(parsed["schema"]))
	assert.JSONEq(t, string(manifest["warnings"]), string(parsed["warnings"]))
}
