// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keptSnapshots is how many snapshots sync keeps when no config sets snapshots.keep.
const keptSnapshots = 12

// oldSnapshots returns n fixtures with year-2000 IDs, one per month, oldest first,
// so every one sorts older than the ID a real sync mints from the clock.
// The oldest is the only one with its own size.
func oldSnapshots(n int) []snapshotFixture { return datedSnapshots(2000, n) }

// newerSnapshots is oldSnapshots in year 2999, so every one sorts newer than a real snapshot.
func newerSnapshots(n int) []snapshotFixture { return datedSnapshots(2999, n) }

func datedSnapshots(year, n int) []snapshotFixture {
	fixtures := make([]snapshotFixture, n)
	for i := range fixtures {
		taken := time.Date(year, time.Month(i+1), 1, 0, 0, 0, 0, time.UTC)
		fixtures[i] = snapshotFixture{
			id:       taken.Format("20060102T150405Z"),
			bytes:    middleBytes,
			taken:    taken,
			source:   homeQuicken,
			verified: true,
		}
	}
	fixtures[0].bytes = oldestBytes
	return fixtures
}

// unreconciledBundle writes a bundle whose one account's ending balance differs from its transactions.
func unreconciledBundle(t *testing.T, dir string) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	account := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: account, EndDate: &day, EndingBalance: "100.01"})
	return b.WriteBundle(t, dir)
}

// snapshotFiles returns the paths in dir ending in suffix.
func snapshotFiles(t *testing.T, dir, suffix string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	require.NoError(t, err)
	return matches
}

func Test_run_sync_deletes_the_snapshot_beyond_the_newest_12_and_prints_a_pruned_line(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".json"))
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots)
	assert.Len(t, snapshotFiles(t, dir, ".json"), keptSnapshots)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}1 snapshot beyond the newest %d \(%s\)\n$`, keptSnapshots, megabytes(oldestBytes)), stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_run_sync_deletes_nothing_when_validation_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, oldSnapshots(keptSnapshots)...)
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	bundle := unreconciledBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 1, exitCode, stderr.String())
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots+1)
	assert.Len(t, snapshotFiles(t, dir, ".json"), keptSnapshots+2)
	assert.FileExists(t, orphan)
	assert.NotContains(t, stdout.String(), "Pruned")
}

// runSyncRemoving runs quarry sync with args under ctx, its Server removing files through remove.
func runSyncRemoving(ctx context.Context, remove func(string) error, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	base := env.NewServer
	env.NewServer = func(ctx context.Context, opts ...snapshot.Option) (*snapshot.Server, error) {
		return base(ctx, append(opts, snapshot.WithRemove(remove))...)
	}

	exitCode := runWith(ctx, append([]string{"sync"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

// syncThenWrite syncs bundle under the test's HOME, writes fixtures beside the snapshot it took,
// and returns that snapshot's ID and the snapshots folder.
func syncThenWrite(t *testing.T, home string, fixtures ...snapshotFixture) (string, string) {
	t.Helper()
	syncBundle(t, v9fixture.OpenBundle(t, filepath.Join(home, "Documents")))
	dir := filepath.Join(storeDirUnder(home), "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, dir, ".sqlite"))
	writeSnapshots(t, home, fixtures...)
	return id, dir
}

// copyOutsideFolder copies id's snapshot and manifest from dir to ~/kept, beyond the folder a test is about to
// make unlistable, and returns the snapshot's path.
func copyOutsideFolder(t *testing.T, home, dir, id string) string {
	t.Helper()
	kept := filepath.Join(home, "kept")
	require.NoError(t, os.MkdirAll(kept, 0o700))
	for _, ext := range []string{".sqlite", ".json"} {
		raw, err := os.ReadFile(filepath.Join(dir, id+ext))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(kept, id+ext), raw, 0o600))
	}
	return filepath.Join(kept, id+".sqlite")
}

// runSyncFrom runs quarry sync --from id with extra args.
func runSyncFrom(t *testing.T, id string, extra ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), append([]string{"sync", "--from", id}, extra...), &stdout, &stderr)
	return exitCode, stdout.String(), stderr.String()
}

func Test_run_sync_from_honours_snapshots_keep_from_config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	older := oldSnapshots(3)
	id, dir := syncThenWrite(t, home, older...)
	writeConfig(t, home, snapshotsKeepConfig(2))

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{filepath.Join(dir, older[2].id+".sqlite"), filepath.Join(dir, id+".sqlite")}, snapshotFiles(t, dir, ".sqlite"))
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}2 snapshots beyond the newest 2 \(%s\)\n$`, megabytes(oldestBytes+middleBytes)), stdout)
}

func Test_run_sync_from_json_carries_a_null_pruned_key_on_a_schema_mismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 1, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	id := snapshotID(onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".sqlite"))

	exitCode, stdout, _ := runSyncFrom(t, id, "--json")

	require.Equal(t, 1, exitCode)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	prunedValue, present := parsed["pruned"]
	require.True(t, present, "the pruned key must be present even when nothing was pruned")
	assert.JSONEq(t, "null", string(prunedValue))
}

func Test_run_sync_warns_when_an_old_snapshot_cannot_be_deleted_and_still_succeeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runSyncRemoving(context.Background(), refusingRemove(fixtures[0].id+".sqlite"), "--quicken", bundle.Dir)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot delete snapshot "+fixtures[0].id+": permission denied; run quarry snapshots prune to try again\n", stderr)
	assert.NotContains(t, stdout, "Pruned")
	assert.FileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
}

func Test_run_sync_prunes_the_snapshots_it_can_and_warns_about_the_one_it_cannot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots + 2)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runSyncRemoving(context.Background(), refusingRemove(fixtures[1].id+".sqlite"), "--quicken", bundle.Dir)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot delete snapshot "+fixtures[1].id+": permission denied; run quarry snapshots prune to try again\n", stderr)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}2 snapshots beyond the newest %d \(%s\)\n$`, keptSnapshots, megabytes(oldestBytes+middleBytes)), stdout)
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
	assert.FileExists(t, filepath.Join(dir, fixtures[1].id+".sqlite"))
	assert.NoFileExists(t, filepath.Join(dir, fixtures[2].id+".sqlite"))
}

func Test_run_sync_from_an_older_snapshot_says_and_the_stores_own(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	newer := newerSnapshots(keptSnapshots)
	older := oldSnapshots(1)
	id, dir := syncThenWrite(t, home, append(newer, older...)...)

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}1 snapshot beyond the newest %d and the store's own \(%s\)\n$`, keptSnapshots, megabytes(oldestBytes)), stdout)
	assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
	assert.NoFileExists(t, filepath.Join(dir, older[0].id+".sqlite"))
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots+1)
}

func Test_run_sync_from_says_the_newest_one_when_snapshots_keep_is_1(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, dir := syncThenWrite(t, home, oldSnapshots(2)...)
	writeConfig(t, home, snapshotsKeepConfig(1))

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}2 snapshots beyond the newest one \(%s\)\n$`, megabytes(oldestBytes+middleBytes)), stdout)
	assert.Equal(t, []string{filepath.Join(dir, id+".sqlite")}, snapshotFiles(t, dir, ".sqlite"))
}

func Test_run_sync_from_warns_when_it_cannot_list_the_snapshots_folder(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, dir := syncThenWrite(t, home)
	kept := copyOutsideFolder(t, home, dir, id)
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runSyncFrom(t, kept)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot list "+snapshotsShown+" to delete old snapshots: permission denied; "+
		"run quarry snapshots prune to try again\n", stderr)
	assert.NotContains(t, stdout, "Pruned")
}
