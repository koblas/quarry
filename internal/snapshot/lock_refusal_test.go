package snapshot_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shownLock  = "~/Library/Application Support/quarry/quarry.lock"
	shownDir   = "~/Library/Application Support/quarry"
	shownAppSp = "~/Library/Application Support"
)

var errNoLocks = errors.New("no locks available")

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file modes")
	}
}

// lockRig is a home directory whose quarry folder holds the lock file under test.
type lockRig struct {
	home     string
	dir      string
	lockPath string
}

// newLockRig returns a rig whose quarry folder exists and is empty.
func newLockRig(t *testing.T) lockRig {
	t.Helper()
	rig := newLockRigWithoutFolder(t)
	require.NoError(t, os.MkdirAll(rig.dir, 0o700))
	return rig
}

// newLockRigWithoutFolder returns a rig whose home is empty: no Library, no quarry folder.
func newLockRigWithoutFolder(t *testing.T) lockRig {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "Library", "Application Support", "quarry")
	return lockRig{home: home, dir: dir, lockPath: filepath.Join(dir, "quarry.lock")}
}

// lockCommand is one way to take the writer lock: a command's mode, its method, and the outcome its refusals promise.
type lockCommand struct {
	name    string
	mode    lockfile.Mode
	acquire func(*snapshot.Server) (func(), error)
	outcome string
}

func lockCommands() []lockCommand {
	return []lockCommand{
		{
			name:    "sync",
			mode:    lockfile.ModeSync,
			acquire: func(s *snapshot.Server) (func(), error) { return s.LockForSync(context.Background()) },
			outcome: "this sync changed nothing",
		},
		{
			name:    "prune",
			mode:    lockfile.ModePrune,
			acquire: func(s *snapshot.Server) (func(), error) { return s.LockForPrune(context.Background()) },
			outcome: "this prune deleted nothing",
		},
	}
}

func (r lockRig) server(mode lockfile.Mode, opts ...lockfile.Option) *snapshot.Server {
	return snapshot.NewServer(snapshot.WithHome(r.home), snapshot.WithLocker(lockfile.New(r.lockPath, mode, opts...)))
}

// requireRefusal fails unless err is a RefusalError saying want, and release is nil.
func requireRefusal(t *testing.T, release func(), err error, want string) {
	t.Helper()
	refusal, ok := errors.AsType[snapshot.RefusalError](err)
	require.True(t, ok, "want a RefusalError, got %v", err)
	assert.Equal(t, want, refusal.Error())
	assert.Nil(t, release)
}

func Test_lock_refuses_a_lock_file_that_is_not_a_regular_file_naming_it(t *testing.T) {
	rows := []struct {
		name    string
		arrange func(t *testing.T, rig lockRig)
	}{
		{"a directory", func(t *testing.T, rig lockRig) {
			t.Helper()
			require.NoError(t, os.Mkdir(rig.lockPath, 0o700))
		}},
		{"a symlink to a regular file", func(t *testing.T, rig lockRig) {
			t.Helper()
			target := filepath.Join(rig.home, "elsewhere.lock")
			require.NoError(t, os.WriteFile(target, nil, 0o600))
			require.NoError(t, os.Symlink(target, rig.lockPath))
		}},
	}
	for _, row := range rows {
		for _, cmd := range lockCommands() {
			t.Run(row.name+" "+cmd.name, func(t *testing.T) {
				rig := newLockRig(t)
				row.arrange(t, rig)

				release, err := cmd.acquire(rig.server(cmd.mode))

				requireRefusal(t, release, err, shownLock+" is not a regular file; remove it, then run the command again")
			})
		}
	}
}

func Test_lock_refuses_a_lock_file_it_cannot_open_naming_its_reason_and_the_fix(t *testing.T) {
	skipAsRoot(t)
	for _, cmd := range lockCommands() {
		t.Run(cmd.name, func(t *testing.T) {
			rig := newLockRig(t)
			require.NoError(t, os.WriteFile(rig.lockPath, nil, 0o000))

			release, err := cmd.acquire(rig.server(cmd.mode))

			requireRefusal(t, release, err, "cannot open "+shownLock+": permission denied; "+
				"make it readable by your user, or remove it, then run the command again")
		})
	}
}

func Test_lock_refuses_a_missing_lock_file_it_cannot_create_naming_its_reason_and_the_fix(t *testing.T) {
	skipAsRoot(t)
	for _, cmd := range lockCommands() {
		t.Run(cmd.name, func(t *testing.T) {
			rig := newLockRig(t)
			require.NoError(t, os.Chmod(rig.dir, 0o500))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(rig.dir, 0o700)) })

			release, err := cmd.acquire(rig.server(cmd.mode))

			requireRefusal(t, release, err, "cannot create "+shownLock+": permission denied; "+
				"make "+shownDir+" writable by your user, then run the command again")
		})
	}
}

func Test_lock_refuses_a_quarry_folder_it_cannot_create_naming_the_folder_to_make_writable(t *testing.T) {
	tests := []struct {
		name       string
		arrange    func(t *testing.T, rig lockRig)
		wantReason string
	}{
		{
			name: "a read-only Library",
			arrange: func(t *testing.T, rig lockRig) {
				t.Helper()
				skipAsRoot(t)
				library := filepath.Join(rig.home, "Library")
				require.NoError(t, os.Mkdir(library, 0o500))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(library, 0o700)) })
			},
			wantReason: "permission denied",
		},
		{
			name: "a file where Library should be",
			arrange: func(t *testing.T, rig lockRig) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(rig.home, "Library"), nil, 0o600))
			},
			wantReason: "not a directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := newLockRigWithoutFolder(t)
			tt.arrange(t, rig)

			release, err := lockCommands()[0].acquire(rig.server(lockfile.ModeSync))

			requireRefusal(t, release, err, "cannot create "+shownDir+": "+tt.wantReason+"; "+
				"make "+shownAppSp+" writable by your user, then run the command again")
		})
	}
}

func Test_lock_refuses_a_lock_that_cannot_be_taken_naming_its_reason_and_the_command_outcome(t *testing.T) {
	faults := []struct {
		name       string
		errno      syscall.Errno
		wantReason string
	}{
		{"flock not supported", syscall.ENOTSUP, "operation not supported"},
		{"no locks available", syscall.ENOLCK, "no locks available"},
	}
	for _, fault := range faults {
		for _, cmd := range lockCommands() {
			t.Run(fault.name+" "+cmd.name, func(t *testing.T) {
				rig := newLockRig(t)
				srv := rig.server(cmd.mode, lockfile.WithFlock(func(int, int) error { return fault.errno }))

				release, err := cmd.acquire(srv)

				requireRefusal(t, release, err, "cannot lock "+shownLock+": "+fault.wantReason+", "+cmd.outcome+"; "+
					shownDir+" must be on a disk that supports file locks")
			})
		}
	}
}

func Test_lock_refusal_prints_an_absolute_path_when_the_lock_is_outside_home(t *testing.T) {
	rig := newLockRig(t)
	require.NoError(t, os.Mkdir(rig.lockPath, 0o700))
	srv := snapshot.NewServer(
		snapshot.WithHome(filepath.Join(t.TempDir(), "someone-else")),
		snapshot.WithLocker(lockfile.New(rig.lockPath, lockfile.ModeSync)),
	)

	release, err := srv.LockForSync(context.Background())

	requireRefusal(t, release, err, rig.lockPath+" is not a regular file; remove it, then run the command again")
}

func Test_lock_returns_an_error_it_cannot_phrase_unchanged(t *testing.T) {
	unrefusable := []struct {
		name string
		err  error
	}{
		{"a plain error", errNoLocks},
		{"a lockfile error of an unknown kind", &lockfile.Error{Kind: lockfile.Kind(0), Path: "quarry.lock", Err: errNoLocks}},
		{"a folder-create error with no cause", &lockfile.Error{Kind: lockfile.KindFolderCreate, Path: "quarry.lock"}},
		{"a create error with no cause", &lockfile.Error{Kind: lockfile.KindCreate, Path: "quarry.lock"}},
		{"an open error with no cause", &lockfile.Error{Kind: lockfile.KindOpen, Path: "quarry.lock"}},
		{"a lock error with no cause", &lockfile.Error{Kind: lockfile.KindLock, Path: "quarry.lock"}},
	}
	for _, row := range unrefusable {
		for _, cmd := range lockCommands() {
			t.Run(row.name+" "+cmd.name, func(t *testing.T) {
				srv := snapshot.NewServer(snapshot.WithLocker(fakeLocker{err: row.err}))

				release, err := cmd.acquire(srv)

				require.Equal(t, row.err, err)
				assert.NotErrorAs(t, err, new(snapshot.RefusalError))
				assert.Nil(t, release)
			})
		}
	}
}
