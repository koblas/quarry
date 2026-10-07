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

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
