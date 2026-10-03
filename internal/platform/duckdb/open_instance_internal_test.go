// White-box: a test leaves a database instance running after its handle is
// closed only by pinning the pool's only connection (db.conn), which no
// exported call does.
package duckdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openBound is how long an open of an idle path may take before a test calls it blocked.
const openBound = 5 * time.Second

func Test_open_read_only_does_not_wait_for_an_instance_an_earlier_open_left_running(t *testing.T) {
	t.Parallel()
	path := newStoreFile(t)
	earlier, err := OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	held, err := earlier.conn.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	require.NoError(t, earlier.Close())

	reopened := openReadOnlyWithin(t, path, openBound)

	var one int
	err = reopened.QueryRows(t.Context(), "SELECT 1", nil, func(scan func(dest ...any) error) error { return scan(&one) })
	require.NoError(t, err)
	assert.Equal(t, 1, one)
}

// newStoreFile creates an empty DuckDB file and returns its path.
func newStoreFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	db, err := Create(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, db.CheckpointClose(t.Context()))
	return path
}

// openReadOnlyWithin opens path read-only, failing the test if the open has not returned within bound.
func openReadOnlyWithin(t *testing.T, path string, bound time.Duration) *DB {
	t.Helper()
	type opened struct {
		db  *DB
		err error
	}
	result := make(chan opened, 1)
	go func() {
		db, err := OpenReadOnly(t.Context(), path)
		result <- opened{db: db, err: err}
	}()
	select {
	case got := <-result:
		require.NoError(t, got.err)
		t.Cleanup(func() { _ = got.db.Close() })
		return got.db
	case <-time.After(bound):
		require.FailNow(t, "open blocked", "OpenReadOnly(%s) had not returned after %v", path, bound)
		return nil
	}
}
