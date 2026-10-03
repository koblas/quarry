// White-box: these tests reach the pool's only connection (db.conn), the
// instance cache each DB owns (db.cache), openOwned and the open lock, none
// of which an exported call exposes.
// Test_open_read_only_does_not_wait_for_an_instance_an_earlier_open_left_running
// is the canary on a driver upgrade: it goes red if the driver stops
// consulting GetInstanceCache.
package duckdb

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/duckdb/duckdb-go/v2/mapping"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openBound is how long an open of an idle path may take before a test calls it blocked.
const openBound = 5 * time.Second

// lockHeldBound is how long a test waits to be sure an open is still blocked behind openMu.
const lockHeldBound = 200 * time.Millisecond

func Test_open_read_only_does_not_wait_for_an_instance_an_earlier_open_left_running(t *testing.T) {
	t.Parallel()
	path := newStoreFile(t)
	earlier, err := OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	held, err := earlier.conn.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	require.NoError(t, earlier.Close())

	reopened := openWithin(t, func() (*DB, error) { return OpenReadOnly(t.Context(), path) }, openBound)

	assert.Equal(t, 1, selectOne(t, reopened))
}

func Test_close_destroys_the_instance_cache_and_a_second_close_is_safe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		open func(ctx context.Context, existing string) (*DB, error)
	}{
		{name: "a read-only open", open: OpenReadOnly},
		{name: "a read-write open", open: OpenReadWrite},
		{name: "a create", open: func(ctx context.Context, existing string) (*DB, error) { return Create(ctx, existing+".new") }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db, err := c.open(t.Context(), newStoreFile(t))
			require.NoError(t, err)
			require.NotNil(t, db.cache.Ptr)

			require.NoError(t, db.Close())

			assert.Nil(t, db.cache.Ptr)
			assert.NoError(t, db.Close())
		})
	}
}

func Test_checkpoint_close_destroys_the_instance_cache(t *testing.T) {
	t.Parallel()
	db, err := Create(t.Context(), filepath.Join(t.TempDir(), "data.duckdb"))
	require.NoError(t, err)
	require.NotNil(t, db.cache.Ptr)

	require.NoError(t, db.CheckpointClose(t.Context()))

	assert.Nil(t, db.cache.Ptr)
}

func Test_an_open_the_driver_refuses_destroys_the_instance_cache(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.duckdb")
	cache := mapping.CreateInstanceCache()
	require.NotNil(t, cache.Ptr)

	db, err := openOwned(t.Context(), &cache, missing+readOnlyDSN, missing)

	require.Error(t, err)
	assert.Nil(t, db)
	assert.Nil(t, cache.Ptr)
}

func Test_an_open_whose_ping_fails_destroys_the_instance_cache(t *testing.T) {
	t.Parallel()
	path := newStoreFile(t)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	cache := mapping.CreateInstanceCache()
	require.NotNil(t, cache.Ptr)

	db, err := openOwned(cancelled, &cache, path+readOnlyDSN, path)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, db)
	assert.Nil(t, cache.Ptr)
}

// Holds the package-wide open lock, so the opens of parallel tests wait for it.
func Test_an_open_waits_while_another_open_holds_the_open_lock(t *testing.T) {
	t.Parallel()
	path := newStoreFile(t)
	var release sync.Once
	unlock := func() { release.Do(openMu.Unlock) }
	openMu.Lock()
	t.Cleanup(unlock)
	result := startOpen(func() (*DB, error) { return OpenReadOnly(t.Context(), path) })

	select {
	case <-result:
		require.FailNow(t, "open did not wait", "OpenReadOnly returned while openMu was held")
	case <-time.After(lockHeldBound):
	}
	unlock()

	assert.Equal(t, 1, selectOne(t, awaitOpen(t, result, openBound)))
}

// Holds the open lock so no parallel open's cache is installed during the call.
func Test_the_driver_uses_its_shared_cache_outside_an_open(t *testing.T) {
	t.Parallel()
	openMu.Lock()
	defer openMu.Unlock()

	first := duckdbdriver.GetInstanceCache()
	second := duckdbdriver.GetInstanceCache()

	assert.NotNil(t, first.Ptr)
	assert.Equal(t, first.Ptr, second.Ptr)
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

// selectOne runs SELECT 1 on db and returns the value read.
func selectOne(t *testing.T, db *DB) int {
	t.Helper()
	var one int
	err := db.QueryRows(t.Context(), "SELECT 1", nil, func(scan func(dest ...any) error) error { return scan(&one) })
	require.NoError(t, err)
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
func awaitOpen(t *testing.T, result <-chan opened, bound time.Duration) *DB {
	t.Helper()
	select {
	case got := <-result:
		require.NoError(t, got.err)
		t.Cleanup(func() { _ = got.db.Close() })
		return got.db
	case <-time.After(bound):
		require.FailNow(t, "open blocked", "open had not returned after %v", bound)
		return nil
	}
}

// openWithin runs open, failing the test if it has not returned within bound.
func openWithin(t *testing.T, open func() (*DB, error), bound time.Duration) *DB {
	t.Helper()
	return awaitOpen(t, startOpen(open), bound)
}
