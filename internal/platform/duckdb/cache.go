package duckdb

import (
	"database/sql"
	"sync"
	"sync/atomic"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/duckdb/duckdb-go/v2/mapping"
)

// The driver opens every file through one process-wide instance cache. Once
// the last handle to a database is closed, the cache's next open of that
// path spins until the instance has shut down, and an instance an
// interrupted query left running never does. Every open therefore gets a
// cache of its own (see openDB), so no open waits on, or shares an instance
// with, any other. Test_open_read_only_does_not_wait_for_an_instance_an_earlier_open_left_running
// is the canary on a driver upgrade: it goes red if the driver stops
// consulting GetInstanceCache.
var (
	// openMu serialises openDB so the driver's lookup below sees the cache of the open that is running.
	openMu sync.Mutex
	// openCache is the cache the open in progress must use, nil outside one.
	openCache atomic.Pointer[mapping.InstanceCache]
)

func init() {
	shared := duckdbdriver.GetInstanceCache
	duckdbdriver.GetInstanceCache = func() mapping.InstanceCache {
		if cache := openCache.Load(); cache != nil {
			return *cache
		}
		return shared()
	}
}

// openDB opens dsn through the driver using cache. The caller owns cache and
// destroys it after closing the returned database. No code outside this
// package may open DuckDB itself: a raw open running beside openDB can be
// handed an open's private cache.
func openDB(dsn string, cache *mapping.InstanceCache) (*sql.DB, error) {
	openMu.Lock()
	defer openMu.Unlock()
	openCache.Store(cache)
	defer openCache.Store(nil)

	connector, err := duckdbdriver.NewConnector(dsn, nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers add the path and operation
	}
	return sql.OpenDB(connector), nil
}
