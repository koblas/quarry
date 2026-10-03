package duckdb

import (
	"database/sql"
	"sync"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/duckdb/duckdb-go/v2/mapping"
)

// The driver opens every file through one process-wide instance cache. Once
// the last handle to a database is closed, the cache's next open of that
// path spins until the instance has shut down, and an instance an
// interrupted query left running never does. A read-only open therefore
// gets a cache of its own (see openDB), so no open waits on, or shares an
// instance with, any other.
var (
	// openMu serialises openDB so the driver's lookup below sees the cache of the open that is running.
	openMu sync.Mutex
	// openCache is the cache the open in progress must use, nil for the driver's shared one; openMu guards it.
	openCache *mapping.InstanceCache
	// sharedCache is the driver's own lookup, replaced in init.
	sharedCache = duckdbdriver.GetInstanceCache
)

func init() {
	duckdbdriver.GetInstanceCache = func() mapping.InstanceCache {
		if openCache != nil {
			return *openCache
		}
		return sharedCache()
	}
}

// openDB opens dsn through the driver using cache, or the driver's shared
// cache when cache is nil. The caller owns cache and destroys it after
// closing the returned database.
func openDB(dsn string, cache *mapping.InstanceCache) (*sql.DB, error) {
	openMu.Lock()
	defer openMu.Unlock()
	openCache = cache
	defer func() { openCache = nil }()

	connector, err := duckdbdriver.NewConnector(dsn, nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers add the path and operation
	}
	return sql.OpenDB(connector), nil
}
