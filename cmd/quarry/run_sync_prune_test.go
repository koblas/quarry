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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
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
	home := newHome(t)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".json"))
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots)
	assert.Len(t, snapshotFiles(t, dir, ".json"), keptSnapshots)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}1 snapshot beyond the newest %d \(%s\)\n$`, keptSnapshots, megabytes(oldestBytes)), stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_run_sync_deletes_nothing_when_validation_fails(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, oldSnapshots(keptSnapshots)...)
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	bundle := unreconciledBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

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
	env.NewServer = serverRemovingThrough(env.NewServer, remove)

	exitCode := runWith(ctx, append([]string{"sync"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

// syncThenWrite syncs bundle under the test's HOME, writes fixtures beside the snapshot it took,
// and returns that snapshot's ID and the snapshots folder.
func syncThenWrite(t *testing.T, home string, fixtures ...snapshotFixture) (string, string) {
	t.Helper()
	syncBundle(t, v9fixture.OpenBundle(t, filepath.Join(home, "Documents")))
	id := soleSnapshotIDUnder(t, home)
	writeSnapshots(t, home, fixtures...)
	return id, snapshotsDirUnder(home)
}

// soleSnapshotIDUnder is the ID of the one snapshot in home's snapshots folder; the test fails if there is not exactly one.
func soleSnapshotIDUnder(t *testing.T, home string) string {
	t.Helper()
	return snapshotID(onlyFileWithSuffix(t, snapshotsDirUnder(home), ".sqlite"))
}

// prunedMember is the raw "pruned" member of sync's --json stdout and whether the key is there at all.
func prunedMember(tb testing.TB, stdout string) (json.RawMessage, bool) {
	tb.Helper()
	var parsed map[string]json.RawMessage
	require.NoError(tb, json.Unmarshal([]byte(stdout), &parsed))
	raw, present := parsed["pruned"]
	return raw, present
}

// copyOutsideFolder links id's snapshot and manifest from dir into ~/kept, beyond the folder a test is about to
// make unlistable, and returns the snapshot's path.
func copyOutsideFolder(t *testing.T, home, dir, id string) string {
	t.Helper()
	kept := filepath.Join(home, "kept")
	require.NoError(t, os.MkdirAll(kept, 0o700))
	hardLink(t, filepath.Join(dir, id+".json"), filepath.Join(kept, id+".json"))
	return hardLink(t, filepath.Join(dir, id+".sqlite"), filepath.Join(kept, id+".sqlite"))
}

// runSyncFrom runs quarry sync --from id with extra args.
func runSyncFrom(t *testing.T, id string, extra ...string) (int, string, string) {
	t.Helper()
	exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sync", "--from", id}, extra...))
	return exitCode, stdout.String(), stderr.String()
}

func Test_run_sync_from_honours_snapshots_keep_from_config(t *testing.T) {
	home := newHome(t)
	older := oldSnapshots(3)
	id, dir := syncThenWrite(t, home, older...)
	writeConfig(t, home, snapshotsKeepConfig(2))

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{filepath.Join(dir, older[2].id+".sqlite"), filepath.Join(dir, id+".sqlite")}, snapshotFiles(t, dir, ".sqlite"))
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}2 snapshots beyond the newest 2 \(%s\)\n$`, megabytes(oldestBytes+middleBytes)), stdout)
}

func Test_run_sync_from_json_carries_a_null_pruned_key_on_a_schema_mismatch(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	syncCode, _, syncStderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})
	require.Equal(t, 1, syncCode, syncStderr.String())
	id := soleSnapshotIDUnder(t, home)

	exitCode, stdout, _ := runSyncFrom(t, id, "--json")

	require.Equal(t, 1, exitCode)
	prunedValue, present := prunedMember(t, stdout)
	require.True(t, present, "the pruned key must be present even when nothing was pruned")
	assert.JSONEq(t, "null", string(prunedValue))
}

func Test_run_sync_warns_when_an_old_snapshot_cannot_be_deleted_and_still_succeeds(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
	id, dir := syncThenWrite(t, home, oldSnapshots(2)...)
	writeConfig(t, home, snapshotsKeepConfig(1))

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Regexp(t, fmt.Sprintf(`Pruned {4}2 snapshots beyond the newest one \(%s\)\n$`, megabytes(oldestBytes+middleBytes)), stdout)
	assert.Equal(t, []string{filepath.Join(dir, id+".sqlite")}, snapshotFiles(t, dir, ".sqlite"))
}

func Test_run_sync_from_warns_when_it_cannot_list_the_snapshots_folder(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
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
	home := newHome(t)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

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
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	pruned, _ := prunedMember(t, stdout.String())
	assert.JSONEq(t, `{"keep": 12, "deleted": [], "failed": []}`, string(pruned))
	assert.Empty(t, stderr.String())
}

func Test_run_sync_json_carries_a_null_pruned_key_when_validation_fails(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, oldSnapshots(keptSnapshots)...)
	bundle := unreconciledBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, _ := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 1, exitCode)
	prunedValue, present := prunedMember(t, stdout.String())
	require.True(t, present, "the pruned key must be present even when validation failed")
	assert.JSONEq(t, "null", string(prunedValue))
	assert.Len(t, snapshotFiles(t, dir, ".sqlite"), keptSnapshots+1)
}

func Test_run_sync_json_lists_a_failed_delete_in_pruned_and_in_warnings(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
	id, dir := syncThenWrite(t, home)
	kept := copyOutsideFolder(t, home, dir, id)
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runSyncFrom(t, kept, "--json")

	require.Equal(t, 0, exitCode, stderr)
	pruned, _ := prunedMember(t, stdout)
	assert.JSONEq(t, `{"keep": 12, "deleted": [], "failed": []}`, string(pruned))
	assert.Equal(t, []string{"cannot list " + dir + " to delete old snapshots: permission denied; run quarry snapshots prune to try again"},
		decodeSyncDoc(t, stdout).Warnings)
	assert.Equal(t, "quarry: warning: cannot list "+snapshotsShown+" to delete old snapshots: permission denied; "+
		"run quarry snapshots prune to try again\n", stderr)
}

func Test_run_sync_json_lists_the_prune_warnings_after_config_and_history_warnings(t *testing.T) {
	home := newHome(t)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	fixtures := oldSnapshots(keptSnapshots)
	writeSnapshots(t, home, fixtures...)

	exitCode, out, errOut := runSyncRemoving(context.Background(), refusingRemove(fixtures[0].id+".sqlite"),
		"--quicken", filepath.Join(home, "Documents", "Home.quicken"), "--json")

	require.Equal(t, 0, exitCode, errOut)
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		combinedCarryWarning,
		"cannot delete snapshot " + fixtures[0].id + ": permission denied; run quarry snapshots prune to try again",
	}, decodeSyncDoc(t, out).Warnings)
}

const interruptedWhilePruningLine = "quarry: sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish\n"

// interruptedRun is what interruptedSync leaves: the fixtures it wrote, their folder and the run's result.
type interruptedRun struct {
	fixtures       []snapshotFixture
	dir            string
	exitCode       int
	stdout, stderr string
}

// interruptedSync runs quarry sync with args beside 14 old snapshots, three beyond the newest 12: the
// newest of them is refused, the next cancels the run once deleted, the oldest is never attempted.
func interruptedSync(t *testing.T, args ...string) interruptedRun {
	t.Helper()
	home := newHome(t)
	fixtures := oldSnapshots(keptSnapshots + 2)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	remove := cancellingRemove(cancel, fixtures[1].id+".json", fixtures[2].id+".sqlite")

	exitCode, stdout, stderr := runSyncRemoving(ctx, remove, append([]string{"--quicken", bundle.Dir}, args...)...)
	return interruptedRun{fixtures: fixtures, dir: dir, exitCode: exitCode, stdout: stdout, stderr: stderr}
}

func Test_run_sync_says_it_was_interrupted_while_deleting_old_snapshots(t *testing.T) {
	got := interruptedSync(t)
	fixtures, stdout, stderr := got.fixtures, got.stdout, got.stderr

	assert.Equal(t, 1, got.exitCode)
	assert.Equal(t, "quarry: warning: cannot delete snapshot "+fixtures[2].id+": permission denied; run quarry snapshots prune to try again\n"+
		interruptedWhilePruningLine, stderr)
	assert.Regexp(t, `(?m)^Store {5}`+regexp.QuoteMeta(storeShown)+`$`, stdout)
	assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 \(`+megabytes(middleBytes)+`\)\n$`, stdout)
	assert.NoFileExists(t, filepath.Join(got.dir, fixtures[1].id+".sqlite"))
	assert.FileExists(t, filepath.Join(got.dir, fixtures[0].id+".sqlite"))
}

func Test_run_sync_json_prints_the_document_when_interrupted_while_deleting_old_snapshots(t *testing.T) {
	got := interruptedSync(t, "--json")
	fixtures := got.fixtures

	assert.Equal(t, 1, got.exitCode)
	doc := decodeSyncDoc(t, got.stdout)
	require.NotNil(t, doc.Pruned)
	require.Len(t, doc.Pruned.Deleted, 1)
	assert.Equal(t, fixtures[1].id, doc.Pruned.Deleted[0].ID)
	require.Len(t, doc.Pruned.Failed, 1)
	assert.Equal(t, fixtures[2].id, doc.Pruned.Failed[0].ID)
	warning := "cannot delete snapshot " + fixtures[2].id + ": permission denied; run quarry snapshots prune to try again"
	assert.Equal(t, []string{warning}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+warning+"\n"+interruptedWhilePruningLine, got.stderr)
}

func Test_run_sync_succeeds_when_interrupted_with_no_snapshot_left_to_delete(t *testing.T) {
	home := newHome(t)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	exitCode, stdout, stderr := runSyncRemoving(ctx, cancellingRemove(cancel, fixtures[0].id+".json", ""), "--quicken", bundle.Dir)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 \(`+megabytes(oldestBytes)+`\)\n$`, stdout)
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
	assert.FileExists(t, orphan)
}
