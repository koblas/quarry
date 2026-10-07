package duckdb_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/require"
)

// writeStoreHolding creates a DuckDB file at path whose table t holds the one value v.
func writeStoreHolding(tb testing.TB, path string, v int32) string {
	tb.Helper()
	db, err := duckdb.Create(tb.Context(), path)
	require.NoError(tb, err)
	_, err = db.Exec(tb.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(tb, err)
	require.NoError(tb, db.AppendRows(tb.Context(), "t", [][]any{{v}}))
	require.NoError(tb, db.CheckpointClose(tb.Context()))
	return path
}

// onlyValue is the one value of db's table t.
func onlyValue(tb testing.TB, db *duckdb.DB) int32 {
	tb.Helper()
	var v int32
	err := db.QueryRows(tb.Context(), "SELECT v FROM t", nil, func(scan func(dest ...any) error) error { return scan(&v) })
	require.NoError(tb, err)
	return v
}

func newOpenDatabase(tb testing.TB) (*duckdb.DB, string) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data.duckdb")
	db, err := duckdb.Create(tb.Context(), path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = db.Close() })
	return db, path
}

// scriptedContext is a context that never fires Done, so the driver's own interrupt
// cannot be what stops a read; its Err is whatever err returns.
type scriptedContext struct {
	err func() error
}

func (scriptedContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (scriptedContext) Done() <-chan struct{}       { return nil }
func (scriptedContext) Value(any) any               { return nil }
func (c scriptedContext) Err() error                { return c.err() }

func newTestTable(tb testing.TB, ddl string) *duckdb.DB {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data.duckdb")
	db, err := duckdb.Create(tb.Context(), path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(tb.Context(), ddl)
	require.NoError(tb, err)
	return db
}

// cancelAfterNErrCalls reports Err() as nil for its first n calls, then as
// context.Canceled — lands AppendRows' per-row check on a chosen row deterministically.
type cancelAfterNErrCalls struct {
	context.Context //nolint:containedctx // wraps a Context to override Err alone

	n     int
	calls int
}

func (c *cancelAfterNErrCalls) Err() error {
	c.calls++
	if c.calls > c.n {
		return context.Canceled
	}
	return nil
}

var errNotDuckDB = errors.New("boom")

// skipAsRoot skips t under root, whom file modes do not stop.
func skipAsRoot(tb testing.TB) {
	tb.Helper()
	if os.Geteuid() == 0 {
		tb.Skip("root ignores file modes")
	}
}

// DuckDB's own wording, capitals included: the predicates and the prefix strip match it exactly.
var (
	errOpenFaultTexts = errors.New("Could not set lock on file; not a valid DuckDB database file") //nolint:staticcheck // DuckDB's capitalised text
	errTwoLines       = errors.New("Parser Error: first\nsecond")                                  //nolint:staticcheck // DuckDB's capitalised type prefix
)

// lockHolderEnv names the database file Test_hold_a_database_open_for_writing holds when run as a child process.
const lockHolderEnv = "QUARRY_DUCKDB_LOCK_HOLDER"

// useZone makes zone the process-local zone and db's TimeZone for the test.
func useZone(tb testing.TB, db *duckdb.DB, zone string) {
	tb.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(tb, err)
	_, err = db.Exec(tb.Context(), "SET GLOBAL TimeZone = '"+zone+"'")
	require.NoError(tb, err)
	//nolint:gosmopolitan // the test swaps the process-local zone; Cleanup restores it
	previous := time.Local
	time.Local = loc                             //nolint:gosmopolitan // restored by Cleanup
	tb.Cleanup(func() { time.Local = previous }) //nolint:gosmopolitan // restores the zone swapped above
}
