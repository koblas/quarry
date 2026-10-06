package snapshot

import (
	"context"
	"errors"

	"github.com/koblas/quarry/internal/platform/lockfile"
)

// lockWords is the per-command wording of a lock refusal: the held line, and the outcome every other refusal promises.
type lockWords struct {
	held    string
	outcome string
}

var (
	syncLockWords = lockWords{
		held: "another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
			"run the command again once that one finishes",
		outcome: "this sync changed nothing",
	}
	pruneLockWords = lockWords{
		held: "another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
			"run the command again once that one finishes",
		outcome: "this prune deleted nothing",
	}
)

// LockForSync takes the writer lock once for a whole sync, auto-prune
// included, and returns the func that releases it; with no Locker it takes
// nothing. A held or unusable lock is a RefusalError; any other failure is
// returned as the Locker gave it.
func (s *Server) LockForSync(ctx context.Context) (func(), error) {
	return s.lock(ctx, syncLockWords)
}

// LockForPrune takes the writer lock once for a whole prune and returns the func that releases it;
// with no Locker, or no quarry folder to hold a lock file, it takes nothing. A held or unusable lock
// is a RefusalError; any other failure is returned as the Locker gave it.
func (s *Server) LockForPrune(ctx context.Context) (func(), error) {
	release, err := s.lock(ctx, pruneLockWords)
	if lockErr, ok := errors.AsType[*lockfile.Error](err); ok && lockErr.Kind == lockfile.KindFolderMissing {
		return func() {}, nil
	}
	return release, err
}

// lock acquires the Locker's lock, turning a failure lockRefusal can phrase into a RefusalError.
func (s *Server) lock(ctx context.Context, words lockWords) (func(), error) {
	if s.locker == nil {
		return func() {}, nil
	}
	release, err := s.locker.Acquire(ctx)
	if lockErr, ok := errors.AsType[*lockfile.Error](err); ok {
		if refusal, ok := lockRefusal(s.home, lockErr, words); ok {
			return nil, refusal
		}
	}
	return release, err //nolint:wrapcheck // errors lockRefusal cannot phrase are returned as given
}
