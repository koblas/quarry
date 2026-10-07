package lockfile_test

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// refusalDeadline bounds a refused Acquire, so a blocking flock fails an assertion, not the go test timeout.
	refusalDeadline = 2 * time.Second
)

var errAcquireWaited = errors.New("acquire waited for the lock")

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// TestMain doubles as the child process the kill test holds a lock in: with
// holderPathEnv set it takes that lock, reports it, and blocks until killed.
func TestMain(m *testing.M) {
	path := os.Getenv(holderPathEnv)
	if path == "" {
		os.Exit(m.Run())
	}
	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	if err != nil {
		os.Exit(3)
	}
	defer release()
	// A collected release drops the flock; collecting now makes that fail the kill test.
	runtime.GC()
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

func Test_acquire_with_an_unknown_mode_never_creates_the_quarry_folder(t *testing.T) {
	path := lockPath(t)

	release, err := lockfile.New(path, lockfile.Mode(99)).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindFolderMissing, lockErr.Kind)
	assert.Nil(t, release)
	assert.NoDirExists(t, filepath.Dir(path))
}

func Test_acquire_refuses_through_a_hard_link_to_the_held_lock_file(t *testing.T) {
	path := lockPath(t)
	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)
	link := filepath.Join(filepath.Dir(path), "other-name.lock")
	require.NoError(t, os.Link(path, link))

	_, err = acquireNow(t, lockfile.New(link, lockfile.ModeSync))

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindHeld, lockErr.Kind)
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

// notRegularRows are lock paths that exist but are not a regular file; each arranges its shape at path.
func notRegularRows() []struct {
	name    string
	arrange func(t *testing.T, path string)
} {
	return []struct {
		name    string
		arrange func(t *testing.T, path string)
	}{
		{name: "directory", arrange: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.Mkdir(path, 0o700))
		}},
		{name: "symlink to a regular file", arrange: func(t *testing.T, path string) {
			t.Helper()
			target := filepath.Join(filepath.Dir(path), "elsewhere")
			require.NoError(t, os.WriteFile(target, nil, 0o600))
			require.NoError(t, os.Symlink(target, path))
		}},
		{name: "dangling symlink", arrange: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(path), "gone"), path))
		}},
		{name: "fifo", arrange: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, syscall.Mkfifo(path, 0o600))
		}},
	}
}

func Test_acquire_refuses_a_lock_file_that_is_not_a_regular_file(t *testing.T) {
	modes := map[string]lockfile.Mode{"sync": lockfile.ModeSync, "prune": lockfile.ModePrune}
	for _, row := range notRegularRows() {
		for modeName, mode := range modes {
			t.Run(row.name+" in "+modeName+" mode", func(t *testing.T) {
				path := lockPath(t)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				row.arrange(t, path)

				release, err := acquireNow(t, lockfile.New(path, mode))

				var lockErr *lockfile.Error
				require.ErrorAs(t, err, &lockErr)
				assert.Equal(t, lockfile.KindNotRegular, lockErr.Kind)
				assert.Equal(t, path, lockErr.Path)
				assert.Nil(t, release)
			})
		}
	}
}

func Test_acquire_classifies_a_folder_that_cannot_be_created(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "Application Support")
	require.NoError(t, os.WriteFile(notAFolder, nil, 0o600))
	path := filepath.Join(notAFolder, "quarry", "quarry.lock")

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindFolderCreate, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	require.ErrorIs(t, err, syscall.ENOTDIR)
	assert.Nil(t, release)
}

func Test_acquire_classifies_a_lock_file_that_exists_but_cannot_be_opened(t *testing.T) {
	skipAsRoot(t)
	path := lockPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Chmod(path, 0o000))

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindOpen, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	require.ErrorIs(t, err, syscall.EACCES)
	assert.Nil(t, release)
}

func Test_acquire_classifies_a_lock_file_that_cannot_be_created_in_a_read_only_folder(t *testing.T) {
	skipAsRoot(t)
	path := lockPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o500))
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })

	release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindCreate, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	require.ErrorIs(t, err, syscall.EACCES)
	assert.Nil(t, release)
}

func Test_acquire_takes_a_lock_file_that_is_read_only(t *testing.T) {
	path := lockPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, nil, 0o400))

	release, err := acquireNow(t, lockfile.New(path, lockfile.ModeSync))

	require.NoError(t, err)
	t.Cleanup(release)
}

func Test_acquire_classifies_another_flock_error_and_closes_the_file(t *testing.T) {
	for _, cause := range []error{syscall.ENOTSUP, syscall.ENOLCK} {
		t.Run(cause.Error(), func(t *testing.T) {
			var lockedFD int
			flock := func(fd, _ int) error {
				lockedFD = fd
				return cause
			}
			path := lockPath(t)

			release, err := lockfile.New(path, lockfile.ModeSync, lockfile.WithFlock(flock)).Acquire(context.Background())

			var lockErr *lockfile.Error
			require.ErrorAs(t, err, &lockErr)
			assert.Equal(t, lockfile.KindLock, lockErr.Kind)
			assert.Equal(t, path, lockErr.Path)
			require.ErrorIs(t, err, cause)
			assert.Nil(t, release)
			assert.ErrorIs(t, syscall.Flock(lockedFD, syscall.LOCK_UN), syscall.EBADF)
		})
	}
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

func Test_acquire_classifies_a_quarry_folder_that_exists_as_a_non_folder(t *testing.T) {
	rows := []struct {
		name    string
		arrange func(t *testing.T, folder string)
	}{
		{"a regular file", func(t *testing.T, folder string) {
			t.Helper()
			require.NoError(t, os.WriteFile(folder, nil, 0o600))
		}},
		{"a symlink to a regular file", func(t *testing.T, folder string) {
			t.Helper()
			target := filepath.Join(filepath.Dir(folder), "elsewhere")
			require.NoError(t, os.WriteFile(target, nil, 0o600))
			require.NoError(t, os.Symlink(target, folder))
		}},
	}
	modes := map[string]lockfile.Mode{"sync": lockfile.ModeSync, "prune": lockfile.ModePrune}
	for _, row := range rows {
		for modeName, mode := range modes {
			t.Run(row.name+" in "+modeName+" mode", func(t *testing.T) {
				path := lockPath(t)
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o700))
				row.arrange(t, filepath.Dir(path))

				release, err := acquireNow(t, lockfile.New(path, mode))

				var lockErr *lockfile.Error
				require.ErrorAs(t, err, &lockErr)
				assert.Equal(t, lockfile.KindNotFolder, lockErr.Kind)
				assert.Equal(t, path, lockErr.Path)
				assert.Nil(t, release)
			})
		}
	}
}

func Test_acquire_in_prune_mode_classifies_a_path_through_a_file_as_not_a_folder(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "Application Support")
	require.NoError(t, os.WriteFile(notAFolder, nil, 0o600))
	path := filepath.Join(notAFolder, "quarry", "quarry.lock")

	release, err := lockfile.New(path, lockfile.ModePrune).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindNotFolder, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	require.ErrorIs(t, err, syscall.ENOTDIR)
	assert.Nil(t, release)
}

func Test_acquire_classifies_a_folder_that_cannot_be_searched_as_a_folder_it_cannot_open(t *testing.T) {
	skipAsRoot(t)
	modes := map[string]lockfile.Mode{"sync": lockfile.ModeSync, "prune": lockfile.ModePrune}
	for modeName, mode := range modes {
		t.Run(modeName, func(t *testing.T) {
			path := lockPath(t)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.Chmod(filepath.Dir(path), 0o600))
			t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })

			release, err := acquireNow(t, lockfile.New(path, mode))

			var lockErr *lockfile.Error
			require.ErrorAs(t, err, &lockErr)
			assert.Equal(t, lockfile.KindFolderOpen, lockErr.Kind)
			assert.Equal(t, path, lockErr.Path)
			require.ErrorIs(t, err, syscall.EACCES)
			assert.Nil(t, release)
		})
	}
}

func Test_acquire_in_prune_mode_classifies_a_symlink_loop_above_the_folder_as_a_folder_it_cannot_open(t *testing.T) {
	loop := filepath.Join(t.TempDir(), "loop")
	require.NoError(t, os.Symlink(loop, loop))
	path := filepath.Join(loop, "quarry", "quarry.lock")

	release, err := lockfile.New(path, lockfile.ModePrune).Acquire(context.Background())

	var lockErr *lockfile.Error
	require.ErrorAs(t, err, &lockErr)
	assert.Equal(t, lockfile.KindFolderOpen, lockErr.Kind)
	assert.Equal(t, path, lockErr.Path)
	require.ErrorIs(t, err, syscall.ELOOP)
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
		{name: "not regular", kind: lockfile.KindNotRegular, want: "lock /q/quarry.lock is not a regular file"},
		{name: "folder create", kind: lockfile.KindFolderCreate, want: "folder of lock /q/quarry.lock cannot be created"},
		{name: "create", kind: lockfile.KindCreate, want: "lock /q/quarry.lock cannot be created"},
		{name: "open", kind: lockfile.KindOpen, want: "lock /q/quarry.lock cannot be opened"},
		{name: "lock", kind: lockfile.KindLock, want: "lock /q/quarry.lock cannot be taken"},
		{name: "not folder", kind: lockfile.KindNotFolder, want: "folder of lock /q/quarry.lock is not a folder"},
		{name: "folder open", kind: lockfile.KindFolderOpen, want: "folder of lock /q/quarry.lock cannot be opened"},
		{name: "unknown", kind: lockfile.Kind(0), want: "lock /q/quarry.lock failed"},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			err := &lockfile.Error{Kind: c.kind, Path: "/q/quarry.lock"}

			assert.EqualError(t, err, c.want)
		})
	}
}
