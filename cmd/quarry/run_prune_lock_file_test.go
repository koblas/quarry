// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireNothingDeleted fails t unless all five snapshots and the orphan manifest are still there.
func (p prunableStore) requireNothingDeleted(t *testing.T) {
	t.Helper()
	requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
	assert.FileExists(t, p.orphan)
}

func Test_run_snapshots_prune_refuses_an_unusable_lock_file_with_a_fix(t *testing.T) {
	for _, row := range unusableLockRows() {
		for _, cell := range outputCells {
			t.Run(row.name+" "+cell.name, func(t *testing.T) {
				p := newPrunableStore(t)
				row.arrange(t, p.home, storeDirUnder(p.home))

				exitCode, stdout, stderr := runPrune(t, append([]string{"--keep", "3"}, cell.flag...)...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, row.wantLine, stderr)
				p.requireNothingDeleted(t)
			})
		}
	}
}

func Test_run_snapshots_prune_lock_refuses_a_quarry_folder_that_is_a_file_naming_the_folder(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			quarryDir := storeDirUnder(home)
			require.NoError(t, os.MkdirAll(filepath.Dir(quarryDir), 0o700))
			require.NoError(t, os.WriteFile(quarryDir, []byte("not a folder"), 0o600))

			exitCode, stdout, stderr := runWithoutConfigFile(t, home, append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: "+quarryDirS+" is not a folder; rename or remove it, then run the command again\n", stderr)
			content, err := os.ReadFile(quarryDir)
			require.NoError(t, err)
			assert.Equal(t, "not a folder", string(content))
		})
	}
}

func Test_run_snapshots_prune_lock_refuses_a_quarry_folder_it_cannot_search_naming_the_folder_and_its_reason(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			skipAsRoot(t)
			p := newPrunableStore(t)
			quarryDir := storeDirUnder(p.home)
			require.NoError(t, os.Chmod(quarryDir, 0o600))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(quarryDir, 0o700)) })

			exitCode, stdout, stderr := runWithoutConfigFile(t, p.home, append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot open "+quarryDirS+": permission denied; "+
				"make it readable and writable by your user, then run the command again\n", stderr)
			require.NoError(t, os.Chmod(quarryDir, 0o700))
			p.requireNothingDeleted(t)
		})
	}
}

// Needs runWith: the flock fault is a Locker option, not a state the filesystem can be put in.
func Test_run_snapshots_prune_refuses_when_the_disk_cannot_take_a_lock_with_a_fix(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			p := newPrunableStore(t)
			var stdout, stderr bytes.Buffer
			env := testEnv(&stdout, &stderr)
			env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
				storeDir := storeDirUnder(p.home)
				notSupported := lockfile.WithFlock(func(int, int) error { return syscall.ENOTSUP })
				return snapshot.NewServer(
					snapshot.WithSnapshotDir(snapshotsDirUnder(p.home)),
					snapshot.WithHome(p.home),
					snapshot.WithStoreProbe(duckstore.New(storeDir)),
					snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune, notSupported)),
				), nil
			}

			exitCode := runWith(context.Background(), append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...), env)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: cannot lock "+lockShown+": operation not supported, so this prune deleted nothing; "+
				quarryDirS+" must be on a disk that supports file locks\n", stderr.String())
			p.requireNothingDeleted(t)
		})
	}
}

// The prune runs on its own goroutine: a lock file that blocks the open must fail the test, not hang it.
func Test_run_snapshots_prune_refuses_a_fifo_lock_file_without_hanging(t *testing.T) {
	type result struct {
		exitCode       int
		stdout, stderr string
	}
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			p := newPrunableStore(t)
			require.NoError(t, syscall.Mkfifo(lockPathUnder(storeDirUnder(p.home)), 0o600))
			done := make(chan result, 1)

			go func() {
				exitCode, stdout, stderr := runPrune(t, append([]string{"--keep", "3"}, cell.flag...)...)
				done <- result{exitCode, stdout, stderr}
			}()

			select {
			case got := <-done:
				assert.Equal(t, 1, got.exitCode)
				assert.Empty(t, got.stdout)
				assert.Equal(t, "quarry: "+lockShown+" is not a regular file; remove it, then run the command again\n", got.stderr)
				p.requireNothingDeleted(t)
			case <-time.After(10 * time.Second):
				t.Fatal("prune did not return: its lock file is a fifo")
			}
		})
	}
}

func Test_run_snapshots_prune_deletes_with_a_read_only_lock_file(t *testing.T) {
	p := newPrunableStore(t)
	require.NoError(t, os.WriteFile(lockPathUnder(storeDirUnder(p.home)), nil, 0o400))

	exitCode, _, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	requireSnapshotsGone(t, p.dir, pruneOldest, pruneMiddle)
	requireSnapshotsKept(t, p.dir, pruneMorning, pruneNoon, pruneNewest)
	assert.NoFileExists(t, p.orphan)
}
