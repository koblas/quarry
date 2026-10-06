package snapshot_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lockHeldSyncRefusal = "another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
	"run the command again once that one finishes"

// newLockedServer returns a Server wired to a real lockfile adapter in a fresh quarry folder.
var errNoLocks = errors.New("no locks available")

func newLockedServer(t *testing.T, opts ...lockfile.Option) *snapshot.Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quarry", "quarry.lock")
	return snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModeSync, opts...)))
}

// newLockedServerPair returns two Servers whose lockers contend for one lock file.
func newLockedServerPair(t *testing.T) (*snapshot.Server, *snapshot.Server) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quarry", "quarry.lock")
	first := snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModeSync)))
	second := snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModeSync)))
	return first, second
}

func Test_lock_for_sync_refuses_with_the_lock_held_line(t *testing.T) {
	holder, contender := newLockedServerPair(t)
	release, err := holder.LockForSync(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)

	_, err = contender.LockForSync(context.Background())

	refusal, ok := errors.AsType[snapshot.RefusalError](err)
	require.True(t, ok, "want a RefusalError, got %v", err)
	assert.Equal(t, lockHeldSyncRefusal, refusal.Error())
}

func Test_lock_for_sync_after_release_locks_again(t *testing.T) {
	holder, contender := newLockedServerPair(t)
	release, err := holder.LockForSync(context.Background())
	require.NoError(t, err)
	release()

	again, err := contender.LockForSync(context.Background())
	require.NoError(t, err)
	t.Cleanup(again)

	_, err = holder.LockForSync(context.Background())
	assert.ErrorAs(t, err, new(snapshot.RefusalError))
}

func Test_lock_for_sync_without_a_locker_is_a_no_op(t *testing.T) {
	srv := snapshot.NewServer()

	release, err := srv.LockForSync(context.Background())

	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func Test_lock_for_sync_returns_other_lock_errors_unchanged(t *testing.T) {
	srv := newLockedServer(t, lockfile.WithFlock(func(int, int) error { return errNoLocks }))

	release, err := srv.LockForSync(context.Background())

	require.ErrorIs(t, err, errNoLocks)
	assert.NotErrorAs(t, err, new(snapshot.RefusalError))
	assert.Nil(t, release)
}
