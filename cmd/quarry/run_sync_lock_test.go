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
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
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
