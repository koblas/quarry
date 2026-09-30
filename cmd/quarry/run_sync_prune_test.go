// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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
func oldSnapshots(n int) []snapshotFixture {
	fixtures := make([]snapshotFixture, n)
	for i := range fixtures {
		taken := time.Date(2000, time.Month(i+1), 1, 0, 0, 0, 0, time.UTC)
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
