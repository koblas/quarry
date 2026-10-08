// White-box: these tests reach the pool's only connection (db.conn), the
// instance cache each DB owns (db.cache), openOwned and the open lock, none
// of which an exported call exposes.
// Test_open_read_only_does_not_wait_for_an_instance_an_earlier_open_left_running
// is the canary on a driver upgrade: it goes red if the driver stops
// consulting GetInstanceCache.
//
// White-box: checkNoWAL is unexported. A real CHECKPOINT always removes the
// .wal file itself, so the only way to exercise "a .wal remains" is to
// plant one beside an already-closed database and call the check directly.
package duckdb

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/duckdb/duckdb-go/v2/mapping"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_no_wal_check_fails_when_a_wal_file_remains(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path+".wal", nil, 0o600))

	err := checkNoWAL(path)

	require.ErrorIs(t, err, ErrWALRemains)
}

func Test_no_wal_check_passes_when_no_wal_file_exists(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")

	err := checkNoWAL(path)

	assert.NoError(t, err)
}

// An unreadable parent directory makes os.Stat fail with EACCES rather than
// ErrNotExist — that error must pass through, not be swallowed as "no WAL".
func Test_no_wal_check_fails_when_the_stat_itself_errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	path := filepath.Join(dir, "data.duckdb")

	err := checkNoWAL(path)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrWALRemains)
}
