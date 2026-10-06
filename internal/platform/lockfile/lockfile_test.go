package lockfile_test

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	holderPathEnv  = "QUARRY_LOCKFILE_TEST_HOLD"
	holderReadyMsg = "locked"
	// refusalDeadline bounds how long a refused Acquire may take: a blocking
	// flock never answers, so the test fails here instead of at the go test timeout.
	refusalDeadline = 2 * time.Second
)

var (
	errAcquireWaited = errors.New("acquire waited for the lock")
	errNoLocks       = errors.New("no locks available")
)

// TestMain doubles as the child process the kill test holds a lock in: with
// holderPathEnv set it takes that lock, reports it, and blocks until killed.
func TestMain(m *testing.M) {
	path := os.Getenv(holderPathEnv)
	if path == "" {
		os.Exit(m.Run())
	}
	_, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	if err != nil {
		os.Exit(3)
	}
	_, _ = os.Stdout.WriteString(holderReadyMsg + "\n")
	select {}
}

// lockPath is the lock file's path under a quarry folder inside the test's temp dir.
func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "Application Support", "quarry", "quarry.lock")
}

func acquireNow(t *testing.T, l *lockfile.Locker) (func(), error) {
	t.Helper()
	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		release, err := l.Acquire(context.Background())
		done <- result{release, err}
	}()
	select {
	case r := <-done:
		return r.release, r.err
	case <-time.After(refusalDeadline):
		require.FailNow(t, "Acquire did not return: it is waiting for the lock instead of refusing")
		return nil, errAcquireWaited
	}
}

func Test_acquire_refuses_at_once_while_another_open_holds_it(t *testing.T) {
	path := lockPath(t)
	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)

	_, err = acquireNow(t, lockfile.New(path, lockfile.ModeSync))

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindHeld, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	assert.ErrorIs(t, err, syscall.EWOULDBLOCK)
}

func Test_acquire_after_release_succeeds(t *testing.T) {
	path := lockPath(t)
	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	release()

	again, err := acquireNow(t, lockfile.New(path, lockfile.ModeSync))

	require.NoError(t, err)
	again()
}

func Test_acquire_release_called_twice_leaves_a_later_holder_alone(t *testing.T) {
	path := lockPath(t)
	first, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	first()
	second, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(second)

	first()

	_, err = acquireNow(t, lockfile.New(path, lockfile.ModeSync))
	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindHeld, lockErr.Kind)
}

func Test_acquire_in_sync_mode_creates_the_folder_0700_and_the_lock_file_0600(t *testing.T) {
	path := lockPath(t)

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	require.NoError(t, err)
	t.Cleanup(release)
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	assert.Zero(t, fileInfo.Size())
}

func Test_acquire_in_prune_mode_never_creates_the_quarry_folder(t *testing.T) {
	path := lockPath(t)

	release, err := lockfile.New(path, lockfile.ModePrune).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindFolderMissing, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	assert.Nil(t, release)
	assert.NoDirExists(t, filepath.Dir(path))
}

func Test_acquire_in_prune_mode_takes_the_lock_when_the_folder_exists(t *testing.T) {
	path := lockPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))

	release, err := lockfile.New(path, lockfile.ModePrune).Acquire(context.Background())

	require.NoError(t, err)
	t.Cleanup(release)
	assert.FileExists(t, path)
	_, err = acquireNow(t, lockfile.New(path, lockfile.ModePrune))
	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindHeld, lockErr.Kind)
}

func Test_acquire_fails_unclassified_when_the_quarry_folder_cannot_be_created(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "Application Support")
	require.NoError(t, os.WriteFile(notAFolder, nil, 0o600))
	path := filepath.Join(notAFolder, "quarry", "quarry.lock")

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	require.ErrorIs(t, err, syscall.ENOTDIR)
	require.ErrorContains(t, err, filepath.Join(notAFolder, "quarry"))
	assert.NotErrorAs(t, err, new(*lockfile.Error))
	assert.Nil(t, release)
}

func Test_acquire_fails_unclassified_when_the_lock_file_cannot_be_opened(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	path := filepath.Join(dir, "quarry.lock")
	require.NoError(t, os.Symlink(target, path))

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	require.ErrorIs(t, err, syscall.ELOOP)
	require.ErrorContains(t, err, path)
	assert.NotErrorAs(t, err, new(*lockfile.Error))
	assert.Nil(t, release)
}

func Test_acquire_returns_another_flock_error_unclassified_and_closes_the_file(t *testing.T) {
	var lockedFD int
	flock := func(fd, _ int) error {
		lockedFD = fd
		return errNoLocks
	}
	path := lockPath(t)

	release, err := lockfile.New(path, lockfile.ModeSync, lockfile.WithFlock(flock)).Acquire(context.Background())

	require.ErrorIs(t, err, errNoLocks)
	require.ErrorContains(t, err, path)
	assert.NotErrorAs(t, err, new(*lockfile.Error))
	assert.Nil(t, release)
	assert.ErrorIs(t, syscall.Flock(lockedFD, syscall.LOCK_UN), syscall.EBADF)
}

func Test_acquire_asks_for_an_exclusive_non_blocking_lock(t *testing.T) {
	var asked int
	flock := func(_, how int) error {
		asked = how
		return nil
	}

	release, err := lockfile.New(lockPath(t), lockfile.ModeSync, lockfile.WithFlock(flock)).Acquire(context.Background())

	require.NoError(t, err)
	t.Cleanup(release)
	assert.Equal(t, syscall.LOCK_EX|syscall.LOCK_NB, asked)
}

func Test_acquire_succeeds_after_the_holder_process_is_killed(t *testing.T) {
	path := lockPath(t)
	child := exec.CommandContext(t.Context(), //nolint:gosec // re-executes this test binary as the lock holder
		os.Args[0], "-test.run=^$")
	child.Env = append(os.Environ(), holderPathEnv+"="+path)
	out, err := child.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill() })
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, holderReadyMsg+"\n", line)

	_, whileAlive := acquireNow(t, lockfile.New(path, lockfile.ModeSync))
	require.NoError(t, child.Process.Kill())
	_ = child.Wait()
	release, afterKill := acquireNow(t, lockfile.New(path, lockfile.ModeSync))

	var lockErr *lockfile.Error
	require.ErrorAs(t, whileAlive, &lockErr)
	assert.Equal(t, lockfile.KindHeld, lockErr.Kind)
	require.NoError(t, afterKill)
	release()
}

func Test_acquire_in_prune_mode_fails_unclassified_when_the_folder_cannot_be_checked(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "Application Support")
	require.NoError(t, os.WriteFile(notAFolder, nil, 0o600))
	path := filepath.Join(notAFolder, "quarry", "quarry.lock")

	release, err := lockfile.New(path, lockfile.ModePrune).Acquire(context.Background())

	require.ErrorIs(t, err, syscall.ENOTDIR)
	assert.NotErrorAs(t, err, new(*lockfile.Error))
	assert.Nil(t, release)
}

func Test_lock_error_names_the_path_for_each_kind(t *testing.T) {
	cells := []struct {
		name string
		kind lockfile.Kind
		want string
	}{
		{name: "held", kind: lockfile.KindHeld, want: "lock /q/quarry.lock is held by another process"},
		{name: "folder missing", kind: lockfile.KindFolderMissing, want: "folder of lock /q/quarry.lock does not exist"},
		{name: "unclassified", kind: lockfile.Kind(0), want: "lock /q/quarry.lock failed"},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			err := &lockfile.Error{Kind: c.kind, Path: "/q/quarry.lock"}

			assert.EqualError(t, err, c.want)
		})
	}
}
