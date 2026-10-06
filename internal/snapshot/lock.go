package snapshot

import (
	"context"
	"errors"

	"github.com/koblas/quarry/internal/platform/lockfile"
)

// lockHeldSyncMsg is the refusal for a sync that finds the writer lock held.
const lockHeldSyncMsg = "another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
	"run the command again once that one finishes"

// LockForSync takes the writer lock once for a whole sync, auto-prune
// included, and returns the func that releases it; with no Locker it takes
// nothing. A held lock is a RefusalError; any other failure is returned as the
// Locker gave it.
func (s *Server) LockForSync(ctx context.Context) (func(), error) {
	if s.locker == nil {
		return func() {}, nil
	}
	release, err := s.locker.Acquire(ctx)
	if lockErr, ok := errors.AsType[*lockfile.Error](err); ok && lockErr.Kind == lockfile.KindHeld {
		return nil, RefusalError{msg: lockHeldSyncMsg}
	}
	return release, err //nolint:wrapcheck // the Locker's other errors are returned as given; their copy is not this method's
}

// LockForPrune takes the writer lock once for a whole prune and returns the func that releases it.
func (s *Server) LockForPrune(_ context.Context) (func(), error) {
	return nil, nil //nolint:nilnil // signature-only stub
}
