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
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_describes_the_store_sync_built(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bundle := writeStatusFixtureBundle(t, home)

	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			TakenAt string `json:"taken_at"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	takenAt, err := time.Parse(time.RFC3339, manifest.Snapshot.TakenAt)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	want := fmt.Sprintf("%-10s%s\n%-10s%s, taken %s (just now)\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Store", abbreviated(t, storePathUnder(home), home),
		"Snapshot", snapshotID(snapshotPath), takenAt.In(time.Local).Format("2006-01-02 15:04 MST"), //nolint:gosmopolitan // status prints the user's local zone
		"Source", abbreviated(t, bundle.Dir, home),
		"Dates", "2026-01-05 to 2026-03-20",
		"Rows", "4 transactions, 4 splits, 2 transfers, 0 payees, 0 categories, 0 tags",
		"Balances", "1 account matches Quicken's last reconciled balance; 1 never reconciled and 1 investment account not checked",
		"Splits", "all 4 transactions equal the sum of their splits",
		"Transfers", "1 paired, 1 one-sided",
	)
	assert.Equal(t, want, stdout.String())
}

func Test_run_status_reports_the_latest_build_when_import_runs_holds_several(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	earlierPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	laterPath := filepath.Join(snapshotsDir, "later-build.sqlite")
	var before bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status"}, &before, &bytes.Buffer{}))
	editStore(t, home, "INSERT INTO import_runs SELECT * REPLACE (2 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
	editStore(t, home, "UPDATE import_runs SET snapshot_path = '"+laterPath+"' WHERE id = 2")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, strings.Replace(before.String(), snapshotID(earlierPath), "later-build", 1), stdout.String())
}

func Test_run_status_refuses_when_home_is_unset(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry status again\n",
		stderr.String())
}

func Test_run_status_reports_a_failed_stdout_write(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

func Test_run_help_prints_quarrys_description(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. Every other command reads that store;
quarry never writes to the Quicken file.`)
	assert.Contains(t, stdout.String(), ""+
		"Available Commands:\n"+
		"  accounts    List accounts with their current balances\n"+
		"  cashflow    Show income, spending and savings rate by month or year\n"+
		"  help        Help about any command\n"+
		"  spend       Show spending by category, payee, tag or month\n"+
		"  sql         Run a read-only SQL query against quarry's store\n"+
		"  status      Show which snapshot the store was built from and what it holds\n"+
		"  sync        Snapshot the open Quicken file and rebuild quarry's store from it\n\n")
}

func Test_run_status_help_describes_the_command_without_needing_home(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, and the checks sync ran when it built the store.

status reads only quarry's store; it never looks at Quicken. Run quarry sync
to bring the store up to date.`)
}

// writeStatusFixtureBundle writes a bundle with reconciled, never-reconciled and
// investment accounts, one paired cross-currency transfer and one one-sided leg.
func writeStatusFixtureBundle(t *testing.T, home string) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	earliest := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	transferDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	depositTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &earliest, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: depositTxn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &earliest, EndingBalance: "100.00"})
	sentTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &transferDay})
	b.Entry(v9fixture.EntryRow{Parent: sentTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	receivedTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "75.00", PostedDate: &transferDay})
	b.Entry(v9fixture.EntryRow{Parent: receivedTxn, Amount: "75.00", QuickenID: 2002, Transfer: "1001"})
	strayTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &latest})
	b.Entry(v9fixture.EntryRow{Parent: strayTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	return b.WriteBundle(t, filepath.Join(home, "Documents"))
}
