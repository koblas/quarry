// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, c.flag...), &stdout, &stderr)

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
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"sync", "--from", "20990101T000000Z"}, c.flag...), &stdout, &stderr)

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
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

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
			home := t.TempDir()
			t.Setenv("HOME", home)
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
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeStatusFixtureBundle(t, home)
			c.before(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

			assert.Equal(t, 0, exitCode, stderr.String())
			assert.NotEmpty(t, stdout.String())
		})
	}
}

func Test_run_sync_creates_the_quarry_folder_0700_and_its_lock_file_0600(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

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
			home := t.TempDir()
			t.Setenv("HOME", home)
			syncBundle(t, writeStatusFixtureBundle(t, home))
			var unlockedOut, unlockedErr bytes.Buffer
			require.Equal(t, 0, run(context.Background(), c.args, &unlockedOut, &unlockedErr), unlockedErr.String())
			release, err := lockfile.New(filepath.Join(storeDirUnder(home), "quarry.lock"), lockfile.ModeSync).Acquire(context.Background())
			require.NoError(t, err)
			t.Cleanup(release)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.NotEmpty(t, unlockedOut.String())
			assert.Equal(t, unlockedOut.String(), stdout.String())
		})
	}
}
