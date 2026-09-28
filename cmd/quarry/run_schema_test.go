// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	_, err = os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	assert.ErrorIs(t, err, os.ErrNotExist)
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

	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"  + table   %s\n  + column  %s.%s\n  + column  %s.%s\n"+
			"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
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
		"Store", abbreviated(t, storePath, home),
		"Rows", "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags",
		"Balances", "no accounts to check; 1 never reconciled",
		"Splits", "no transactions to check",
		"Transfers", "none",
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
	manifestBytes, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, string(manifest["snapshot"]), string(parsed["snapshot"]))
	assert.JSONEq(t, string(manifest["schema"]), string(parsed["schema"]))
	assert.JSONEq(t, string(manifest["warnings"]), string(parsed["warnings"]))

	storeValue, present := parsed["store"]
	require.True(t, present, "the store key must be present even when the import was not attempted")
	assert.JSONEq(t, "null", string(storeValue))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(parsed["schema"], &schema))
	assert.Equal(t, false, schema["verified"])
	assert.NotEmpty(t, schema["missing_tables"])
	assert.NotEmpty(t, schema["missing_columns"])
	assert.NotEmpty(t, schema["unexpected_columns"])

	require.NotEmpty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: schema check failed:"))
}

// The MissingSchemaBundle fixture's own dropped table and columns are what
// makes its snapshot mismatch the real embedded reference, independent of
// Quicken: --from re-checks that same snapshot file against the same
// reference and finds the same mismatch.
func Test_run_reports_a_schema_mismatch_with_from(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 1, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", id}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	assert.Equal(t, syncStdout.String(), stdout.String())
	assert.Equal(t, "quarry: schema check failed: snapshot "+id+" of Home.quicken is missing 1 table and 2 columns "+
		"that the schema reference expects; quarry cannot import it until its schema reference is updated\n",
		stderr.String())
	_, err := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func Test_run_reports_a_schema_mismatch_with_from_as_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 1, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", id, "--json"}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	storeValue, present := parsed["store"]
	require.True(t, present, "the store key must be present even when the import was not attempted")
	assert.JSONEq(t, "null", string(storeValue))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(parsed["schema"], &schema))
	assert.Equal(t, false, schema["verified"])
	assert.NotEmpty(t, schema["missing_tables"])
	assert.NotEmpty(t, schema["missing_columns"])
	assert.NotEmpty(t, schema["unexpected_columns"])

	require.NotEmpty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: schema check failed: snapshot "+id))
}

// On the mismatch path no build is reached, so a stdout write failure still
// names the manifest already on disk, not --from --json.
func Test_run_keeps_the_snapshot_message_when_writing_stdout_fails_on_a_schema_mismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
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
