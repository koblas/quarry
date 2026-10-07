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
	"slices"
	"strings"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_stamps_the_store_with_its_format_and_the_build(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	sentTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: sentTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	receivedTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "75.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: receivedTxn, Amount: "75.00", QuickenID: 2002, Transfer: "1001"})
	strayTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: strayTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer
	before := time.Now().UTC().Truncate(time.Microsecond)

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	after := time.Now().UTC()
	require.Equal(t, 0, exitCode, stderr.String())
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			Source  string `json:"source"`
			TakenAt string `json:"taken_at"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	takenAt, err := time.Parse(time.RFC3339, manifest.Snapshot.TakenAt)
	require.NoError(t, err)

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var infoRows, formatVersion int
	var quarryVersion string
	var builtAt, finishedAt time.Time
	require.NoError(t, db.QueryRows(t.Context(), "SELECT count(*) FROM store_info", nil, func(scan func(dest ...any) error) error {
		return scan(&infoRows)
	}))
	require.Equal(t, 1, infoRows)
	require.NoError(t, db.QueryRows(t.Context(), "SELECT format_version, quarry_version, built_at FROM store_info", nil,
		func(scan func(dest ...any) error) error { return scan(&formatVersion, &quarryVersion, &builtAt) }))
	require.NoError(t, db.QueryRows(t.Context(), "SELECT finished_at FROM import_runs", nil,
		func(scan func(dest ...any) error) error { return scan(&finishedAt) }))
	assert.Equal(t, duckstore.FormatVersion, formatVersion)
	assert.NotEmpty(t, quarryVersion)
	assert.False(t, builtAt.Before(before))
	assert.False(t, builtAt.After(after))
	assert.False(t, builtAt.Before(finishedAt))

	var snapshotTakenAt time.Time
	var sourcePath string
	require.NoError(t, db.QueryRows(t.Context(), "SELECT snapshot_taken_at, source_path FROM import_runs", nil,
		func(scan func(dest ...any) error) error { return scan(&snapshotTakenAt, &sourcePath) }))
	assert.Equal(t, takenAt.UTC(), snapshotTakenAt.UTC())
	assert.Equal(t, manifest.Snapshot.Source, sourcePath)
	assert.Equal(t, map[string]string{"1": "2 1 1 1 2"}, stringMap(t, db,
		"SELECT CAST(id AS VARCHAR), concat_ws(' ', balances_never_reconciled, investment_accounts, transfers_paired, "+
			"transfers_cross_currency, transfers_rows) FROM import_runs"))
}

const (
	// betweenLinkID is a valid snapshot ID newer than pruneOldest and older than every other fixture.
	betweenLinkID = "20260928T000000Z"
	// futureLinkID is a valid snapshot ID newer than any a sync mints and older than newerSnapshots'.
	futureLinkID = "29980101T000000Z"
	// absentID is a valid snapshot ID no fixture carries.
	absentID = "20260801T120000Z"
)

// snapshotFile is the path of id's snapshot in dir.
func snapshotFile(dir, id string) string { return filepath.Join(dir, id+".sqlite") }

// hardLink makes link a second name for the file at target and returns link.
func hardLink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Link(target, link))
	return link
}

// symlink makes link a symbolic link to target and returns link.
func symlink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Symlink(target, link))
	return link
}

// fileNames returns the name of every file in dir, sorted.
func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

// symlinkWithManifest symlinks home's latest.sqlite to id's snapshot in dir, spelling its extension as ext, puts
// its manifest beside the link as latest.json so sync --from accepts it, and returns the link.
func symlinkWithManifest(t *testing.T, home, dir, id, ext string) string {
	t.Helper()
	hardLink(t, filepath.Join(dir, id+".json"), filepath.Join(home, "latest.json"))
	return symlink(t, filepath.Join(dir, id+ext), filepath.Join(home, "latest.sqlite"))
}

func Test_run_snapshots_prune_run_twice_never_deletes_the_snapshot_the_recorded_path_resolves_to(t *testing.T) {
	recorded := []string{pruneOldest + ".json", pruneOldest + ".sqlite"}
	newest := []string{pruneNewest + ".json", pruneNewest + ".sqlite"}
	recordedAndNewest := slices.Concat(recorded, newest)
	withNewerLink := slices.Concat(recorded, []string{betweenLinkID + ".sqlite"}, newest)
	withOlderLink := slices.Concat([]string{linkID + ".sqlite"}, recorded, newest)
	cases := []struct {
		name string
		// recorded arranges links around the five snapshots in dir and returns the path the store recorded.
		recorded func(t *testing.T, home, dir string) string
		left     []string
	}{
		{
			name:     "a snapshot in the folder",
			recorded: func(_ *testing.T, _, dir string) string { return snapshotFile(dir, pruneOldest) },
			left:     recordedAndNewest,
		},
		{
			name: "a snapshot in the folder named in another letter case, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return snapshotFile(dir, strings.ToLower(pruneOldest))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink outside the folder to a snapshot named with an upper-case extension, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, filepath.Join(dir, pruneOldest+".SQLITE"), filepath.Join(home, "latest.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink outside the folder whose target has a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, snapshotFile(dir, pruneOldest), filepath.Join(home, "latest.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a symlink in the folder under an older id whose target has a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return symlink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
			},
			left: slices.Concat([]string{linkID + ".sqlite"}, withNewerLink),
		},
		{
			name: "a snapshot in the folder with a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return snapshotFile(dir, pruneOldest)
			},
			left: withNewerLink,
		},
		{
			name: "a snapshot in the folder with an older hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
				return snapshotFile(dir, pruneOldest)
			},
			left: withOlderLink,
		},
		{
			name: "a hard link outside the folder under no snapshot's id keeps every snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, betweenLinkID))
				return hardLink(t, snapshotFile(dir, pruneOldest), filepath.Join(home, "linked.sqlite"))
			},
			left: withNewerLink,
		},
		{
			name: "a path that is gone whose id is a snapshot's",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), pruneOldest)
			},
			left: recordedAndNewest,
		},
		{
			name: "a path that is gone whose id is no snapshot's keeps only the newest",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), absentID)
			},
			left: []string{pruneNewest + ".json", pruneNewest + ".sqlite"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, c.recorded(t, home, dir))

			firstExit, _, firstStderr := runPrune(t, "--keep", "1")
			secondExit, _, secondStderr := runPrune(t, "--keep", "1")

			require.Equal(t, 0, firstExit, firstStderr)
			require.Equal(t, 0, secondExit, secondStderr)
			assert.Equal(t, c.left, fileNames(t, dir))
		})
	}
}

// storeBuiltFromASymlink syncs, rebuilds the store --from a symlink to that snapshot spelled with ext and adds a newer
// hard link of it beside one newer snapshot; it returns the snapshot's ID, the newer snapshot's ID and the folder.
func storeBuiltFromASymlink(t *testing.T, home, ext string) (string, string, string) {
	t.Helper()
	newer := newerSnapshots(1)
	id, dir := syncThenWrite(t, home, newer...)
	exitCode, _, stderr := runSyncFrom(t, symlinkWithManifest(t, home, dir, id, ext))
	require.Equal(t, 0, exitCode, stderr)
	hardLink(t, snapshotFile(dir, id), snapshotFile(dir, futureLinkID))
	return id, newer[0].id, dir
}

func Test_run_snapshots_json_marks_the_target_of_the_symlink_sync_built_the_store_from(t *testing.T) {
	home := newHome(t)
	id, _, _ := storeBuiltFromASymlink(t, home, ".sqlite")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, _ := storeIdentity(t, stdout)
	assert.Equal(t, []string{id}, marked)
}

func Test_run_snapshots_json_marks_the_target_of_a_symlink_that_spells_its_extension_in_upper_case(t *testing.T) {
	home := newHome(t)
	skipOnCaseSensitiveVolume(t, home)
	id, _, _ := storeBuiltFromASymlink(t, home, ".SQLITE")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, _ := storeIdentity(t, stdout)
	assert.Equal(t, []string{id}, marked)
}

func Test_run_snapshots_prune_run_twice_keeps_the_target_of_the_symlink_sync_built_the_store_from(t *testing.T) {
	home := newHome(t)
	id, newerID, dir := storeBuiltFromASymlink(t, home, ".sqlite")

	firstExit, _, firstStderr := runPrune(t, "--keep", "1")
	secondExit, _, secondStderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, firstExit, firstStderr)
	require.Equal(t, 0, secondExit, secondStderr)
	assert.Equal(t,
		[]string{id + ".json", id + ".sqlite", futureLinkID + ".sqlite", newerID + ".json", newerID + ".sqlite"}, fileNames(t, dir))
}

func Test_run_sync_from_keeps_the_snapshot_it_names_and_its_newer_hard_link_beyond_the_newest_12(t *testing.T) {
	cases := []struct {
		name string
		// from arranges a way to name id's snapshot in dir and returns the --from value.
		from func(t *testing.T, home, dir, id string) string
	}{
		{
			name: "a symlink to the snapshot",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				return symlinkWithManifest(t, home, dir, id, ".sqlite")
			},
		},
		{
			name: "a symlink to the snapshot named with an upper-case extension",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				return symlinkWithManifest(t, home, dir, id, ".SQLITE")
			},
		},
		{
			name: "the snapshot's path with an upper-case extension",
			from: func(t *testing.T, home, dir, id string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				return filepath.Join(dir, id+".SQLITE")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			older := oldSnapshots(1)
			id, dir := syncThenWrite(t, home, append(newerSnapshots(keptSnapshots), older...)...)
			from := c.from(t, home, dir, id)
			hardLink(t, snapshotFile(dir, id), snapshotFile(dir, futureLinkID))

			exitCode, stdout, stderr := runSyncFrom(t, from)

			require.Equal(t, 0, exitCode, stderr)
			assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 and the store's own \(`, stdout)
			requireSnapshotsKept(t, dir, id)
			assert.FileExists(t, snapshotFile(dir, futureLinkID))
			assert.NoFileExists(t, snapshotFile(dir, older[0].id))
		})
	}
}

func Test_run_snapshots_prune_keeps_the_snapshot_sync_was_built_from_under_another_extension_once_that_file_is_gone(t *testing.T) {
	home := newHome(t)
	newer := newerSnapshots(1)
	id, dir := syncThenWrite(t, home, newer...)
	backup := filepath.Join(home, "backup")
	require.NoError(t, os.MkdirAll(backup, 0o700))
	from := hardLink(t, snapshotFile(dir, id), filepath.Join(backup, id+".db"))
	hardLink(t, filepath.Join(dir, id+".json"), from+".json")
	exitCode, _, stderr := runSyncFrom(t, from)
	require.Equal(t, 0, exitCode, stderr)
	require.NoError(t, os.Remove(from))
	require.NoError(t, os.Remove(from+".json"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Nothing to delete: 2 snapshots, within the newest one and "+id+", the store's snapshot\n", stdout)
	requireSnapshotsKept(t, dir, id, newer[0].id)
}

// linkID is a valid snapshot ID older than every fixture, given to a link of the store's snapshot.
const linkID = "20200101T000000Z"

// storeIdentity returns, from stdout's snapshots --json document, the id of each entry marked
// "store": true and the id under "store_snapshot".
func storeIdentity(t *testing.T, stdout string) ([]string, string) {
	t.Helper()
	var doc struct {
		Snapshots []struct {
			ID    string `json:"id"`
			Store bool   `json:"store"`
		} `json:"snapshots"`
		StoreSnapshot struct {
			ID string `json:"id"`
		} `json:"store_snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	marked := []string{}
	for _, entry := range doc.Snapshots {
		if entry.Store {
			marked = append(marked, entry.ID)
		}
	}
	return marked, doc.StoreSnapshot.ID
}

func Test_run_snapshots_prune_dry_run_lists_neither_the_recorded_snapshot_nor_its_hard_link(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, filepath.Join(dir, pruneOldest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 3 snapshots (2.6 MB), keeping the newest one and 20260927T143005Z, the store's snapshot:\n"+
		"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
		"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
	assert.FileExists(t, filepath.Join(dir, linkID+".sqlite"))
}

func Test_run_snapshots_json_marks_only_the_recorded_snapshot_when_another_is_a_hard_link_to_it(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, filepath.Join(dir, pruneOldest+".sqlite"))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, storeSnapshotID := storeIdentity(t, stdout)
	assert.Equal(t, []string{pruneOldest}, marked)
	assert.Equal(t, pruneOldest, storeSnapshotID)
}

func Test_run_snapshots_prune_json_lists_neither_the_recorded_snapshot_nor_its_hard_link(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, filepath.Join(dir, pruneOldest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 3,\n"+
		pruneStoreSnapshotJSON(dir, pruneOldest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneMiddle, middleBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	assert.FileExists(t, snapshotFile(dir, linkID))
}

func Test_run_snapshots_prune_says_nothing_to_delete_when_only_a_hard_link_of_the_stores_snapshot_lies_beyond_the_newest_n(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, pruneFixture(pruneNewest, newestBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)))
	hardLink(t, snapshotFile(dir, pruneNewest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, snapshotFile(dir, pruneNewest))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Nothing to delete: 2 snapshots, within the newest one\n", stdout)
	assert.FileExists(t, snapshotFile(dir, linkID))
}

// The store partial and its wal are backdated past the sweep's age gate; a
// fresh one could belong to another build still in flight.
func Test_run_removes_stale_store_leftovers_before_syncing(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(storeDir, 0o700))

	partial := filepath.Join(storeDir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(partial, []byte("crash debris"), 0o600))
	partialWAL := partial + ".wal"
	require.NoError(t, os.WriteFile(partialWAL, []byte("wal"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(partial, old, old))
	require.NoError(t, os.Chtimes(partialWAL, old, old))
	staleWAL := filepath.Join(storeDir, "quarry.duckdb.wal")
	require.NoError(t, os.WriteFile(staleWAL, []byte("wal"), 0o600))

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	_, err := os.Stat(partial)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(partialWAL)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(staleWAL)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(storeDir, "quarry.duckdb"))
	assert.NoError(t, err)
}

// faultDB wraps the real partial-file connection and injects at most one
// fault; with no fault configured it passes every call through.
type faultDB struct {
	duckstore.DB

	path            string
	duplicateTable  string
	checkpointFault error
	afterCheckpoint func()
}

// AppendRows appends rows to duplicateTable twice, so the second append
// fails on the table's real primary key.
func (f *faultDB) AppendRows(ctx context.Context, table string, rows [][]any) error {
	if err := f.DB.AppendRows(ctx, table, rows); err != nil || table != f.duplicateTable {
		return err
	}
	return f.DB.AppendRows(ctx, table, rows)
}

// CheckpointClose returns checkpointFault wrapped as duckdb.CheckpointClose
// wraps a driver error, or runs the real one and then afterCheckpoint.
func (f *faultDB) CheckpointClose(ctx context.Context) error {
	if f.checkpointFault != nil {
		return fmt.Errorf("checkpoint %s: %w", f.path, f.checkpointFault)
	}
	err := f.DB.CheckpointClose(ctx)
	if f.afterCheckpoint != nil {
		f.afterCheckpoint()
	}
	return err
}

// withFault makes the store build its partial file through f over a real
// DuckDB connection.
func withFault(f *faultDB) duckstore.Option {
	return duckstore.WithCreate(func(ctx context.Context, path string) (duckstore.DB, error) {
		db, err := duckdb.Create(ctx, path)
		if err != nil {
			return nil, err
		}
		f.DB, f.path = db, path
		return f, nil
	})
}

func Test_run_never_replaces_the_store_when_the_build_fails(t *testing.T) {
	cases := []struct {
		name       string
		arrange    func(t *testing.T, storeDir string, cancel context.CancelFunc) *faultDB
		wantStderr func(storeDir, storePath, id string) string
	}{
		{
			name: "unwritable store directory",
			arrange: func(t *testing.T, storeDir string, _ context.CancelFunc) *faultDB {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.Mkdir(filepath.Join(storeDir, "snapshots"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(storeDir, "quarry.lock"), nil, 0o600))
				require.NoError(t, os.Chmod(storeDir, 0o500))
				t.Cleanup(func() { _ = os.Chmod(storeDir, 0o700) })
				return &faultDB{}
			},
			wantStderr: func(storeDir, _, _ string) string {
				return "quarry: cannot write to " + storeDir + ": permission denied; make the directory writable by your user\n"
			},
		},
		{
			name: "disk full",
			arrange: func(*testing.T, string, context.CancelFunc) *faultDB {
				return &faultDB{checkpointFault: &duckdbdriver.Error{
					Type: duckdbdriver.ErrorTypeIO, Msg: `IO Error: Could not write file "quarry.duckdb.partial": No space left on device`,
				}}
			},
			wantStderr: func(storeDir, _, id string) string {
				return "quarry: cannot write the store to " + storeDir +
					": no space left on device; free disk space, then run quarry sync --from " + id + "\n"
			},
		},
		{
			name: "other DuckDB error",
			arrange: func(*testing.T, string, context.CancelFunc) *faultDB {
				return &faultDB{duplicateTable: "accounts"}
			},
			wantStderr: func(storeDir, _, id string) string {
				return "quarry: cannot build the store in " + storeDir + ": database/sql/driver: could not close appender: " +
					`Failed to append: Duplicate key "id: acct-1" violates primary key constraint.; run quarry sync --from ` + id + "\n"
			},
		},
		{
			name: "a detection fault",
			arrange: func(*testing.T, string, context.CancelFunc) *faultDB {
				return &faultDB{duplicateTable: "findings"}
			},
			wantStderr: func(storeDir, _, id string) string {
				return "quarry: cannot build the store in " + storeDir + ": database/sql/driver: could not close appender: " +
					`Failed to append: Duplicate key "id: uncategorized:no-payee" violates primary key constraint.; run quarry sync --from ` + id + "\n"
			},
		},
		{
			name: "SIGINT before the swap",
			arrange: func(_ *testing.T, _ string, cancel context.CancelFunc) *faultDB {
				return &faultDB{afterCheckpoint: cancel}
			},
			wantStderr: func(_, storePath, id string) string {
				return "quarry: sync interrupted while building the store; " + storePath +
					" was not changed; run quarry sync --from " + id + " to rebuild it\n"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
			require.NoError(t, os.MkdirAll(storeDir, 0o700))
			storePath := filepath.Join(storeDir, "quarry.duckdb")
			sentinel := []byte("previous store bytes, untouched by a failed build")
			require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
			b := v9fixture.NewBuilder()
			chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
			bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			fault := c.arrange(t, storeDir, cancel)
			var stdout, stderr bytes.Buffer

			env := testEnv(&stdout, &stderr)
			env.NewServer = newServerFactory(fixedRates(), withFault(fault))

			exitCode := runWith(ctx, []string{"sync", "--quicken", bundle.Dir}, env)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			id := snapshotID(onlyFileWithSuffix(t, filepath.Join(storeDir, "snapshots"), ".sqlite"))
			assert.Equal(t,
				c.wantStderr(abbreviated(t, storeDir, home), abbreviated(t, storePath, home), id),
				stderr.String())
			entries, err := os.ReadDir(storeDir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb", "quarry.lock", "snapshots"}, entryNames(entries))
			after, err := os.ReadFile(storePath)
			require.NoError(t, err)
			assert.Equal(t, sentinel, after)
		})
	}
}
