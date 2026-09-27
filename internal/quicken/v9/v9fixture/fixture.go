// Package v9fixture builds a synthetic Quicken Classic for Mac v9 bundle for
// tests: a WAL-mode database held open by a second connection with
// uncheckpointed writes, the state Quicken leaves its file in while it has
// it open.
package v9fixture

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// Bundle is a fixture .quicken bundle: a live database held open with
// uncheckpointed WAL writes.
type Bundle struct {
	Dir                 string // the .quicken bundle directory
	DataPath            string // Dir/data
	CheckpointedAccount string // account name present in DataPath's bytes
	WALOnlyAccount      string // account name present only in the WAL
}

// OpenBundle creates a Home.quicken bundle under dir with ReferenceDDL's
// schema, one account checkpointed into data and a second account written
// only to the WAL. The holder connection that keeps the WAL open stays open
// until tb.Cleanup, so DataPath's -wal and -shm files are present the whole
// test.
func OpenBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	holder, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	holder.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = holder.Close() })

	requireExec(tb, ctx, holder, "PRAGMA journal_mode=WAL")
	requireExec(tb, ctx, holder, "PRAGMA wal_autocheckpoint=0")
	requireExec(tb, ctx, holder, v9.ReferenceDDL)

	const checkpointed = "Checking"
	const walOnly = "WAL Only Savings"
	requireExec(tb, ctx, holder, "INSERT INTO ZACCOUNT (ZNAME) VALUES (?)", checkpointed)
	requireExec(tb, ctx, holder, "PRAGMA wal_checkpoint(PASSIVE)")
	requireExec(tb, ctx, holder, "INSERT INTO ZACCOUNT (ZNAME) VALUES (?)", walOnly)

	return Bundle{
		Dir:                 bundleDir,
		DataPath:            dataPath,
		CheckpointedAccount: checkpointed,
		WALOnlyAccount:      walOnly,
	}
}

func requireExec(tb testing.TB, ctx context.Context, db *sql.DB, query string, args ...any) {
	tb.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	require.NoError(tb, err)
}
