// Package lockfile takes quarry's single-writer lock: an exclusive,
// non-blocking advisory flock on a lock file beside the store. The kernel
// drops the lock when its holder exits, so a killed run leaves nothing stale.
package lockfile
