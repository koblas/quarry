package duckdb

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// openBound is how long an open of an idle path may take before a test calls it blocked.
const openBound = 5 * time.Second

// lockHeldBound is how long a test waits to be sure an open is still blocked behind openMu.
const lockHeldBound = 200 * time.Millisecond

// newStoreFile creates an empty DuckDB file and returns its path.
func newStoreFile(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data.duckdb")
	db, err := Create(tb.Context(), path)
	require.NoError(tb, err)
	require.NoError(tb, db.CheckpointClose(tb.Context()))
	return path
}

// selectOne runs SELECT 1 on db and returns the value read.
func selectOne(tb testing.TB, db *DB) int {
	tb.Helper()
	var one int
	err := db.QueryRows(tb.Context(), "SELECT 1", nil, func(scan func(dest ...any) error) error { return scan(&one) })
	require.NoError(tb, err)
	return one
}

// opened is the outcome of an open started by startOpen.
type opened struct {
	db  *DB
	err error
}

// startOpen runs open in a goroutine and returns the channel its outcome arrives on.
func startOpen(open func() (*DB, error)) <-chan opened {
	result := make(chan opened, 1)
	go func() {
		db, err := open()
		result <- opened{db: db, err: err}
	}()
	return result
}

// awaitOpen waits up to bound for result, failing the test if the open has not returned or failed; the DB is closed on cleanup.
func awaitOpen(tb testing.TB, result <-chan opened, bound time.Duration) *DB {
	tb.Helper()
	select {
	case got := <-result:
		require.NoError(tb, got.err)
		tb.Cleanup(func() { _ = got.db.Close() })
		return got.db
	case <-time.After(bound):
		require.FailNow(tb, "open blocked", "open had not returned after %v", bound)
		return nil
	}
}

// openWithin runs open, failing the test if it has not returned within bound.
func openWithin(tb testing.TB, open func() (*DB, error), bound time.Duration) *DB {
	tb.Helper()
	return awaitOpen(tb, startOpen(open), bound)
}

var (
	errOtherDriverFault   = errors.New("database/sql/driver: API error: invalid input")
	errIndexPastTheResult = errors.New("database/sql/driver: API error: unsupported data type: VARIANT: index: 1")
	errIndexTooLarge      = errors.New("unsupported data type: VARIANT: index: 99999999999999999999")
)
