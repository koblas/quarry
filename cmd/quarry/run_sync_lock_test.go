// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockHeldSyncLine is the refusal a sync prints while another writer holds the lock.
const lockHeldSyncLine = "quarry: another quarry sync or quarry snapshots prune is running, so this sync changed nothing; " +
	"run the command again once that one finishes\n"

// lockedStore is a quarry folder under the test's HOME that holds a sentinel store
// and a lock another writer holds until the test ends.
type lockedStore struct {
	home      string
	quarryDir string
	storePath string
	sentinel  []byte
}

// holdLockedStore points HOME at a fresh quarry folder holding a sentinel store, and holds its lock.
func holdLockedStore(t *testing.T) lockedStore {
	t.Helper()
	home := newHome(t)
	quarryDir := storeDirUnder(home)
	require.NoError(t, os.MkdirAll(quarryDir, 0o700))
	storePath := filepath.Join(quarryDir, "quarry.duckdb")
	sentinel := []byte("previous store bytes, untouched while another writer holds the lock")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))

	release, err := lockfile.New(filepath.Join(quarryDir, "quarry.lock"), lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)

	return lockedStore{home: home, quarryDir: quarryDir, storePath: storePath, sentinel: sentinel}
}

// requireUntouched fails t unless no snapshot was taken and the store holds its sentinel bytes.
func (l lockedStore) requireUntouched(t *testing.T) {
	t.Helper()
	assert.NoDirExists(t, filepath.Join(l.quarryDir, "snapshots"))
	after, err := os.ReadFile(l.storePath)
	require.NoError(t, err)
	assert.Equal(t, l.sentinel, after)
}

func Test_run_sync_refuses_while_another_writer_holds_the_lock(t *testing.T) {
	cells := []struct {
		name string
		flag []string
	}{
		{name: "text", flag: nil},
		{name: "json", flag: []string{"--json"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			locked := holdLockedStore(t)
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			bundle := b.WriteBundle(t, filepath.Join(locked.home, "Documents"))

			exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, c.flag...))

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, lockHeldSyncLine, stderr.String())
			locked.requireUntouched(t)
		})
	}
}

func Test_run_sync_from_refuses_on_the_lock_before_resolving_the_snapshot(t *testing.T) {
	cells := []struct {
		name string
		flag []string
	}{
		{name: "text", flag: nil},
		{name: "json", flag: []string{"--json"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			locked := holdLockedStore(t)

			exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sync", "--from", "20990101T000000Z"}, c.flag...))

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, lockHeldSyncLine, stderr.String())
			locked.requireUntouched(t)
		})
	}
}

func Test_run_sync_refuses_on_the_lock_before_looking_for_the_quicken_file(t *testing.T) {
	cells := []struct {
		name       string
		config     string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "no quicken file anywhere", args: []string{"sync"}, wantCode: 1, wantStderr: lockHeldSyncLine},
		{
			name: "a malformed config is refused first", config: "quicken.path = 12\n", args: []string{"sync"}, wantCode: 1,
			wantStderr: "quarry: " + configShown + ": quicken.path must be a path in quotes, got 12" + configFix + "\n",
		},
		{
			name: "an extra argument is refused first", args: []string{"sync", "extra"}, wantCode: 2,
			wantStderr: "quarry: sync takes no arguments; pass the file with --quicken <path>\n",
		},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			locked := holdLockedStore(t)
			writeConfig(t, locked.home, c.config)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, c.wantCode, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// recordingLocker counts the locks taken and released through it, taking none.
type recordingLocker struct {
	acquired, released int
}

func (r *recordingLocker) Acquire(context.Context) (func(), error) {
	r.acquired++
	return func() { r.released++ }, nil
}

func Test_run_sync_releases_the_lock_when_it_returns(t *testing.T) {
	cells := []struct {
		name     string
		arrange  func(t *testing.T, home string)
		wantCode int
	}{
		{name: "a successful sync", arrange: func(t *testing.T, home string) {
			t.Helper()
			writeStatusFixtureBundle(t, home)
		}, wantCode: 0},
		{name: "a refusal after the lock is taken", arrange: func(*testing.T, string) {}, wantCode: 1},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			c.arrange(t, home)
			locker := &recordingLocker{}
			env := testEnv(io.Discard, io.Discard)
			build := newServerFactory(fixedRates())
			env.NewServer = func(ctx context.Context, opts ...snapshot.Option) (*snapshot.Server, error) {
				return build(ctx, append(opts, snapshot.WithLocker(locker))...)
			}

			exitCode := runWith(context.Background(), []string{"sync"}, env)

			assert.Equal(t, c.wantCode, exitCode)
			assert.Equal(t, 1, locker.acquired)
			assert.Equal(t, 1, locker.released)
		})
	}
}

func Test_run_sync_proceeds_past_a_lock_left_by_an_earlier_run(t *testing.T) {
	cells := []struct {
		name   string
		before func(t *testing.T, home string)
	}{
		{name: "an earlier sync in the same process", before: func(t *testing.T, _ string) {
			t.Helper()
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(context.Background(), []string{"sync"}, &stdout, &stderr), stderr.String())
		}},
		{name: "a lock file left by an acquire and release", before: func(t *testing.T, home string) {
			t.Helper()
			path := filepath.Join(storeDirUnder(home), "quarry.lock")
			release, err := lockfile.New(path, lockfile.ModeSync).Acquire(context.Background())
			require.NoError(t, err)
			release()
		}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			writeStatusFixtureBundle(t, home)
			c.before(t, home)

			exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync"})

			assert.Equal(t, 0, exitCode, stderr.String())
			assert.NotEmpty(t, stdout.String())
		})
	}
}

func Test_run_sync_creates_the_quarry_folder_0700_and_its_lock_file_0600(t *testing.T) {
	home := newHome(t)
	writeStatusFixtureBundle(t, home)

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync"})

	require.Equal(t, 0, exitCode, stderr.String())
	dirInfo, err := os.Stat(storeDirUnder(home))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	lockInfo, err := os.Stat(filepath.Join(storeDirUnder(home), "quarry.lock"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), lockInfo.Mode().Perm())
	assert.Zero(t, lockInfo.Size())
}

func Test_run_status_reports_while_a_sync_holds_the_lock(t *testing.T) {
	cells := []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status"}},
		{name: "sql", args: []string{"sql", "SELECT name FROM v_account_balances ORDER BY source_id"}},
		{name: "snapshots", args: []string{"snapshots"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			syncBundle(t, writeStatusFixtureBundle(t, home))
			var unlockedOut, unlockedErr bytes.Buffer
			require.Equal(t, 0, run(context.Background(), c.args, &unlockedOut, &unlockedErr), unlockedErr.String())
			release, err := lockfile.New(filepath.Join(storeDirUnder(home), "quarry.lock"), lockfile.ModeSync).Acquire(context.Background())
			require.NoError(t, err)
			t.Cleanup(release)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.NotEmpty(t, unlockedOut.String())
			assert.Equal(t, unlockedOut.String(), stdout.String())
		})
	}
}

const (
	lockShown  = "~/Library/Application Support/quarry/quarry.lock"
	quarryDirS = "~/Library/Application Support/quarry"
)

// unusableLockRow arranges the quarry folder under home so its lock file is unusable.
type unusableLockRow struct {
	name     string
	arrange  func(t *testing.T, home, quarryDir string)
	wantLine string
}

func unusableLockRows() []unusableLockRow {
	return []unusableLockRow{
		{
			name: "a directory",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(lockPathUnder(quarryDir), 0o700))
			},
			wantLine: "quarry: " + lockShown + " is not a regular file; remove it, then run the command again\n",
		},
		{
			name: "a symlink to a regular file",
			arrange: func(t *testing.T, home, quarryDir string) {
				t.Helper()
				target := filepath.Join(home, "elsewhere.lock")
				require.NoError(t, os.WriteFile(target, nil, 0o600))
				require.NoError(t, os.Symlink(target, lockPathUnder(quarryDir)))
			},
			wantLine: "quarry: " + lockShown + " is not a regular file; remove it, then run the command again\n",
		},
		{
			name: "a file with mode 0000",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.WriteFile(lockPathUnder(quarryDir), nil, 0o000))
			},
			wantLine: "quarry: cannot open " + lockShown + ": permission denied; " +
				"make it readable by your user, or remove it, then run the command again\n",
		},
		{
			name: "a read-only folder with no lock file",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.Chmod(quarryDir, 0o500))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(quarryDir, 0o700)) })
			},
			wantLine: "quarry: cannot create " + lockShown + ": permission denied; " +
				"make " + quarryDirS + " writable by your user, then run the command again\n",
		},
	}
}

// newSentinelStore points HOME at a quarry folder holding a sentinel store and no lock file.
func newSentinelStore(t *testing.T) lockedStore {
	t.Helper()
	home := newHome(t)
	quarryDir := storeDirUnder(home)
	require.NoError(t, os.MkdirAll(quarryDir, 0o700))
	sentinel := []byte("previous store bytes, untouched while the lock file is unusable")
	storePath := filepath.Join(quarryDir, "quarry.duckdb")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	return lockedStore{home: home, quarryDir: quarryDir, storePath: storePath, sentinel: sentinel}
}

func Test_run_sync_refuses_an_unusable_lock_file_with_a_fix(t *testing.T) {
	for _, row := range unusableLockRows() {
		for _, cell := range outputCells {
			t.Run(row.name+" "+cell.name, func(t *testing.T) {
				locked := newSentinelStore(t)
				writeStatusFixtureBundle(t, locked.home)
				row.arrange(t, locked.home, locked.quarryDir)

				exitCode, stdout, stderr := runQuarry(t, append([]string{"sync"}, cell.flag...)...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, row.wantLine, stderr)
				locked.requireUntouched(t)
			})
		}
	}
}

func Test_run_sync_refuses_with_a_fix_when_it_cannot_create_the_folder_for_its_lock(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			skipAsRoot(t)
			home := newHome(t)
			writeStatusFixtureBundle(t, home)
			library := filepath.Join(home, "Library")
			require.NoError(t, os.Mkdir(library, 0o500))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(library, 0o700)) })

			exitCode, stdout, stderr := runQuarry(t, append([]string{"sync"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot create "+quarryDirS+": permission denied; "+
				"make ~/Library/Application Support writable by your user, then run the command again\n", stderr)
			assert.NoDirExists(t, storeDirUnder(home))
		})
	}
}

// runWithoutConfigFile runs quarry with the config loader reading no file: config.toml lives in the
// quarry folder, so the real loader refuses a broken folder before the lock is reached.
func runWithoutConfigFile(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.LoadConfig = func(string) (config.Config, error) {
		return config.Load(home, filepath.Join(home, "absent.toml"))
	}
	exitCode := runWith(context.Background(), args, env)
	return exitCode, stdout.String(), stderr.String()
}

func Test_run_sync_lock_refuses_a_quarry_folder_that_is_a_file_naming_the_folder(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			home := newHome(t)
			writeStatusFixtureBundle(t, home)
			quarryDir := storeDirUnder(home)
			require.NoError(t, os.MkdirAll(filepath.Dir(quarryDir), 0o700))
			require.NoError(t, os.WriteFile(quarryDir, []byte("not a folder"), 0o600))

			exitCode, stdout, stderr := runWithoutConfigFile(t, home, append([]string{"sync"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: "+quarryDirS+" is not a folder; rename or remove it, then run the command again\n", stderr)
			content, err := os.ReadFile(quarryDir)
			require.NoError(t, err)
			assert.Equal(t, "not a folder", string(content))
		})
	}
}

func Test_run_sync_lock_refuses_a_quarry_folder_it_cannot_search_naming_the_folder_and_its_reason(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			skipAsRoot(t)
			locked := newSentinelStore(t)
			writeStatusFixtureBundle(t, locked.home)
			require.NoError(t, os.Chmod(locked.quarryDir, 0o600))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(locked.quarryDir, 0o700)) })

			exitCode, stdout, stderr := runWithoutConfigFile(t, locked.home, append([]string{"sync"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot open "+quarryDirS+": permission denied; "+
				"make it readable and writable by your user, then run the command again\n", stderr)
			require.NoError(t, os.Chmod(locked.quarryDir, 0o700))
			locked.requireUntouched(t)
		})
	}
}

// Needs runWith: the flock fault is a Locker option, not a state the filesystem can be put in.
func Test_run_sync_refuses_when_the_disk_cannot_take_a_lock_with_a_fix(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			locked := newSentinelStore(t)
			writeStatusFixtureBundle(t, locked.home)
			var stdout, stderr bytes.Buffer
			env := testEnv(&stdout, &stderr)
			build := env.NewServer
			notSupported := lockfile.WithFlock(func(int, int) error { return syscall.ENOTSUP })
			env.NewServer = func(ctx context.Context, opts ...snapshot.Option) (*snapshot.Server, error) {
				locker := lockfile.New(lockPathUnder(locked.quarryDir), lockfile.ModeSync, notSupported)
				return build(ctx, append(opts, snapshot.WithLocker(locker))...)
			}

			exitCode := runWith(context.Background(), append([]string{"sync"}, cell.flag...), env)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: cannot lock "+lockShown+": operation not supported, so this sync changed nothing; "+
				quarryDirS+" must be on a disk that supports file locks\n", stderr.String())
			locked.requireUntouched(t)
		})
	}
}

// The sync runs on its own goroutine: a lock file that blocks the open must fail the test, not hang it.
func Test_run_sync_refuses_a_fifo_lock_file_without_hanging(t *testing.T) {
	type result struct {
		exitCode       int
		stdout, stderr string
	}
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			locked := newSentinelStore(t)
			writeStatusFixtureBundle(t, locked.home)
			require.NoError(t, syscall.Mkfifo(lockPathUnder(locked.quarryDir), 0o600))
			done := make(chan result, 1)

			go func() {
				exitCode, stdout, stderr := runQuarry(t, append([]string{"sync"}, cell.flag...)...)
				done <- result{exitCode, stdout, stderr}
			}()

			select {
			case got := <-done:
				assert.Equal(t, 1, got.exitCode)
				assert.Empty(t, got.stdout)
				assert.Equal(t, "quarry: "+lockShown+" is not a regular file; remove it, then run the command again\n", got.stderr)
				locked.requireUntouched(t)
			case <-time.After(10 * time.Second):
				t.Fatal("sync did not return: its lock file is a fifo")
			}
		})
	}
}

func Test_run_sync_locks_a_read_only_lock_file_and_leaves_its_mode(t *testing.T) {
	locked := newSentinelStore(t)
	writeStatusFixtureBundle(t, locked.home)
	lockPath := lockPathUnder(locked.quarryDir)
	require.NoError(t, os.WriteFile(lockPath, nil, 0o400))

	exitCode, _, stderr := runQuarry(t, "sync")

	require.Equal(t, 0, exitCode, stderr)
	info, err := os.Stat(lockPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o400), info.Mode().Perm())
}
