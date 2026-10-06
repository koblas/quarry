// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockHeldPruneLine is the refusal a prune prints while another writer holds the lock.
const lockHeldPruneLine = "quarry: another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
	"run the command again once that one finishes\n"

// prunableStore is a quarry folder under the test's HOME with five snapshots, a store built from
// the newest, and one orphan manifest: what `prune --keep 3` deletes when nothing stops it.
type prunableStore struct {
	home   string
	dir    string
	orphan string
}

// newPrunableStore points HOME at a fresh prunableStore.
func newPrunableStore(t *testing.T) prunableStore {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	return prunableStore{home: home, dir: dir, orphan: orphan}
}

// holdLock takes the writer lock as a running sync would, and returns the func that releases it.
func (p prunableStore) holdLock(t *testing.T) func() {
	t.Helper()
	release, err := lockfile.New(lockPathUnder(storeDirUnder(p.home)), lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)
	return release
}

func Test_run_snapshots_prune_refuses_while_another_writer_holds_the_lock(t *testing.T) {
	cells := []struct {
		name string
		flag []string
	}{
		{name: "text", flag: nil},
		{name: "json", flag: []string{"--json"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			p := newPrunableStore(t)
			p.holdLock(t)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"snapshots", "prune", "--keep", "3"}, c.flag...), &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, lockHeldPruneLine, stderr.String())
			requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
			assert.FileExists(t, p.orphan)
		})
	}
}

func Test_run_snapshots_prune_proceeds_past_a_lock_left_by_an_earlier_run(t *testing.T) {
	p := newPrunableStore(t)
	release := p.holdLock(t)
	release()

	exitCode, _, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	requireSnapshotsGone(t, p.dir, pruneOldest, pruneMiddle)
	requireSnapshotsKept(t, p.dir, pruneMorning, pruneNoon, pruneNewest)
	assert.NoFileExists(t, p.orphan)
}
