package snapshot_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lockHeldSyncRefusal = "another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
	"run the command again once that one finishes"

const lockHeldPruneRefusal = "another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
	"run the command again once that one finishes"

var errNoLocks = errors.New("no locks available")

// fakeLocker is a Locker whose Acquire fails with err.
type fakeLocker struct{ err error }

func (f fakeLocker) Acquire(context.Context) (func(), error) { return nil, f.err }

// newLockedServer returns a Server wired to a real lockfile adapter in a fresh quarry folder.
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

// newSyncAndPruneServers returns a sync-mode and a prune-mode Server contending for one lock file whose folder exists.
func newSyncAndPruneServers(t *testing.T) (*snapshot.Server, *snapshot.Server) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Mkdir(dir, 0o700))
	path := filepath.Join(dir, "quarry.lock")
	syncSrv := snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModeSync)))
	pruneSrv := snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModePrune)))
	return syncSrv, pruneSrv
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

func Test_lock_for_sync_returns_a_folder_missing_lock_error_unchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quarry", "quarry.lock")
	srv := snapshot.NewServer(snapshot.WithLocker(lockfile.New(path, lockfile.ModePrune)))

	release, err := srv.LockForSync(context.Background())

	lockErr, ok := errors.AsType[*lockfile.Error](err)
	require.True(t, ok, "want the *lockfile.Error as given, got %v", err)
	assert.Equal(t, lockfile.KindFolderMissing, lockErr.Kind)
	assert.NotErrorAs(t, err, new(snapshot.RefusalError))
	assert.Nil(t, release)
}

func Test_lock_for_sync_returns_other_lock_errors_unchanged(t *testing.T) {
	srv := newLockedServer(t, lockfile.WithFlock(func(int, int) error { return errNoLocks }))

	release, err := srv.LockForSync(context.Background())

	require.ErrorIs(t, err, errNoLocks)
	assert.NotErrorAs(t, err, new(snapshot.RefusalError))
	assert.Nil(t, release)
}

func Test_lock_for_prune_refuses_with_the_prune_lock_held_line(t *testing.T) {
	holder, contender := newSyncAndPruneServers(t)
	release, err := holder.LockForSync(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)

	_, err = contender.LockForPrune(context.Background())

	refusal, ok := errors.AsType[snapshot.RefusalError](err)
	require.True(t, ok, "want a RefusalError, got %v", err)
	assert.Equal(t, lockHeldPruneRefusal, refusal.Error())
	assert.NotEqual(t, lockHeldSyncRefusal, refusal.Error())
}

func Test_lock_for_prune_after_release_locks_again(t *testing.T) {
	holder, contender := newSyncAndPruneServers(t)
	release, err := holder.LockForSync(context.Background())
	require.NoError(t, err)
	release()

	again, err := contender.LockForPrune(context.Background())
	require.NoError(t, err)
	t.Cleanup(again)

	_, err = holder.LockForSync(context.Background())
	assert.ErrorAs(t, err, new(snapshot.RefusalError))
}

func Test_lock_for_prune_proceeds_unlocked_when_the_quarry_folder_is_missing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "quarry")
	srv := snapshot.NewServer(snapshot.WithLocker(lockfile.New(filepath.Join(dir, "quarry.lock"), lockfile.ModePrune)))

	release, err := srv.LockForPrune(context.Background())

	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	assert.NoDirExists(t, dir)
}

func Test_lock_for_prune_returns_other_lock_errors_unchanged(t *testing.T) {
	flockFault := newPruneServerInExistingFolder(t, lockfile.WithFlock(func(int, int) error { return errNoLocks }))
	unknownKind := &lockfile.Error{Kind: lockfile.Kind(0), Path: "quarry.lock", Err: errNoLocks}
	tests := []struct {
		name string
		srv  *snapshot.Server
		want error
	}{
		{"the adapter's wrapped flock failure", flockFault, errNoLocks},
		{"a plain error", snapshot.NewServer(snapshot.WithLocker(fakeLocker{err: errNoLocks})), errNoLocks},
		{"a lockfile error of an unclassified kind", snapshot.NewServer(snapshot.WithLocker(fakeLocker{err: unknownKind})), unknownKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, err := tt.srv.LockForPrune(context.Background())

			require.ErrorIs(t, err, tt.want)
			assert.NotErrorAs(t, err, new(snapshot.RefusalError))
			assert.Nil(t, release)
		})
	}
}

func Test_lock_for_prune_without_a_locker_is_a_no_op(t *testing.T) {
	srv := snapshot.NewServer()

	release, err := srv.LockForPrune(context.Background())

	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

// newPruneServerInExistingFolder returns a prune-mode Server whose quarry folder exists.
func newPruneServerInExistingFolder(t *testing.T, opts ...lockfile.Option) *snapshot.Server {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Mkdir(dir, 0o700))
	return snapshot.NewServer(snapshot.WithLocker(lockfile.New(filepath.Join(dir, "quarry.lock"), lockfile.ModePrune, opts...)))
}
