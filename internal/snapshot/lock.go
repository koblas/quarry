package snapshot

import (
	"context"
	"errors"

	"github.com/koblas/quarry/internal/platform/lockfile"
)

const (
	lockHeldSyncMsg = "another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
		"run the command again once that one finishes"
	lockHeldPruneMsg = "another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
		"run the command again once that one finishes"
)

// LockForSync takes the writer lock once for a whole sync, auto-prune
// included, and returns the func that releases it; with no Locker it takes
// nothing. A held lock is a RefusalError; any other failure is returned as the
// Locker gave it.
func (s *Server) LockForSync(ctx context.Context) (func(), error) {
	return s.lock(ctx, lockHeldSyncMsg)
}

// LockForPrune takes the writer lock once for a whole prune and returns the func that releases it;
// with no Locker, or no quarry folder to hold a lock file, it takes nothing. A held lock is a
// RefusalError; any other failure is returned as the Locker gave it.
func (s *Server) LockForPrune(ctx context.Context) (func(), error) {
	release, err := s.lock(ctx, lockHeldPruneMsg)
	if lockErr, ok := errors.AsType[*lockfile.Error](err); ok && lockErr.Kind == lockfile.KindFolderMissing {
		return func() {}, nil
	}
	return release, err
}

// lock acquires the Locker's lock, turning a held lock into a RefusalError saying heldMsg.
func (s *Server) lock(ctx context.Context, heldMsg string) (func(), error) {
	if s.locker == nil {
		return func() {}, nil
	}
	release, err := s.locker.Acquire(ctx)
	if lockErr, ok := errors.AsType[*lockfile.Error](err); ok && lockErr.Kind == lockfile.KindHeld {
		return nil, RefusalError{msg: heldMsg}
	}
	return release, err //nolint:wrapcheck // the Locker's other errors are returned as given; their copy is not this method's
}
