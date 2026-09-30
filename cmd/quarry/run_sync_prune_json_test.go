// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prunedDoc is the "pruned" object of sync's --json document.
type prunedDoc struct {
	Keep    int `json:"keep"`
	Deleted []struct {
		ID    string `json:"id"`
		Path  string `json:"path"`
		Bytes int64  `json:"bytes"`
	} `json:"deleted"`
	Failed []struct {
		ID     string `json:"id"`
		Path   string `json:"path"`
		Reason string `json:"reason"`
	} `json:"failed"`
}

// syncDoc is sync's --json document, its pruned and warnings members decoded.
type syncDoc struct {
	Pruned   *prunedDoc `json:"pruned"`
	Warnings []string   `json:"warnings"`
}

// decodeSyncDoc decodes stdout as sync's --json document.
func decodeSyncDoc(t *testing.T, stdout string) syncDoc {
	t.Helper()
	var doc syncDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc
}

// topLevelKeys returns the keys of the JSON object in raw, in the order written.
func topLevelKeys(t *testing.T, raw string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(raw))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, key.(string)) //nolint:forcetypeassert // an object's keys decode as strings
		var skipped json.RawMessage
		require.NoError(t, dec.Decode(&skipped))
	}
	return keys
}

func Test_run_sync_json_lists_what_it_pruned_after_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, []string{"snapshot", "schema", "store", "pruned", "warnings"}, topLevelKeys(t, stdout.String()))
	pruned := decodeSyncDoc(t, stdout.String()).Pruned
	require.NotNil(t, pruned)
	assert.Equal(t, keptSnapshots, pruned.Keep)
	require.Len(t, pruned.Deleted, 1)
	assert.Equal(t, fixtures[0].id, pruned.Deleted[0].ID)
	assert.Equal(t, filepath.Join(dir, fixtures[0].id+".sqlite"), pruned.Deleted[0].Path)
	assert.Equal(t, int64(oldestBytes), pruned.Deleted[0].Bytes)
	assert.Empty(t, pruned.Failed)
}

func Test_run_sync_json_carries_empty_pruned_lists_when_nothing_was_deleted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, `{"keep": 12, "deleted": [], "failed": []}`, string(parsed["pruned"]))
	assert.Empty(t, stderr.String())
}

func Test_run_sync_json_carries_a_null_pruned_key_when_validation_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, oldSnapshots(keptSnapshots)...)
	bundle := unreconciledBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	prunedValue, present := parsed["pruned"]
	require.True(t, present, "the pruned key must be present even when validation failed")
	assert.JSONEq(t, "null", string(prunedValue))
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots+1)
}

func Test_run_sync_json_lists_a_failed_delete_in_pruned_and_in_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	warning := "cannot delete snapshot " + fixtures[0].id + ": permission denied; run quarry snapshots prune to try again"

	exitCode, stdout, stderr := runSyncRemoving(context.Background(), refusingRemove(fixtures[0].id+".sqlite"), "--quicken", bundle.Dir, "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := decodeSyncDoc(t, stdout)
	require.NotNil(t, doc.Pruned)
	assert.Empty(t, doc.Pruned.Deleted)
	require.Len(t, doc.Pruned.Failed, 1)
	assert.Equal(t, fixtures[0].id, doc.Pruned.Failed[0].ID)
	assert.Equal(t, filepath.Join(dir, fixtures[0].id+".sqlite"), doc.Pruned.Failed[0].Path)
	assert.Equal(t, "permission denied", doc.Pruned.Failed[0].Reason)
	assert.Equal(t, []string{warning}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+warning+"\n", stderr)
}

func Test_run_sync_from_json_names_the_unlistable_folder_by_its_absolute_path_in_warnings(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, dir := syncThenWrite(t, home)
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runSyncFrom(t, id, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.JSONEq(t, `{"keep": 12, "deleted": [], "failed": []}`, string(parsed["pruned"]))
	assert.Equal(t, []string{"cannot list " + dir + " to delete old snapshots: permission denied; run quarry snapshots prune to try again"},
		decodeSyncDoc(t, stdout).Warnings)
	assert.Equal(t, "quarry: warning: cannot list "+snapshotsShown+" to delete old snapshots: permission denied; "+
		"run quarry snapshots prune to try again\n", stderr)
}

func Test_run_sync_json_lists_the_prune_warnings_after_config_transfer_and_history_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &stdout, &stderr), stderr.String())
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a database"), 0o600))
	writeConfig(t, home, "snapshot.keep = 3\n")
	fixtures := oldSnapshots(keptSnapshots)
	writeSnapshots(t, home, fixtures...)

	exitCode, out, errOut := runSyncRemoving(context.Background(), refusingRemove(fixtures[0].id+".sqlite"),
		"--quicken", filepath.Join(home, "Documents", "Home.quicken"), "--json")

	require.Equal(t, 0, exitCode, errOut)
	assert.Equal(t, []string{
		configShown + ": unknown key snapshot.keep; quarry ignores it",
		"1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer",
		"cannot carry import history forward from the previous store (the file is not a DuckDB database); import_runs starts again with this sync",
		"cannot delete snapshot " + fixtures[0].id + ": permission denied; run quarry snapshots prune to try again",
	}, decodeSyncDoc(t, out).Warnings)
}
