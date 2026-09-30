// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const interruptedWhilePruningLine = "quarry: sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish\n"

// interruptedRun is what interruptedSync leaves: the fixtures it wrote, their folder and the run's result.
type interruptedRun struct {
	fixtures       []snapshotFixture
	dir            string
	exitCode       int
	stdout, stderr string
}

// interruptedSync runs quarry sync with args beside 14 old snapshots, three beyond the newest 12: the
// newest of them is refused, the next cancels the run once deleted, the oldest is never attempted.
func interruptedSync(t *testing.T, args ...string) interruptedRun {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots + 2)
	dir := writeSnapshots(t, home, fixtures...)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	remove := cancellingRemove(cancel, fixtures[1].id+".json", fixtures[2].id+".sqlite")

	exitCode, stdout, stderr := runSyncRemoving(ctx, remove, append([]string{"--quicken", bundle.Dir}, args...)...)
	return interruptedRun{fixtures: fixtures, dir: dir, exitCode: exitCode, stdout: stdout, stderr: stderr}
}

func Test_run_sync_says_it_was_interrupted_while_deleting_old_snapshots(t *testing.T) {
	got := interruptedSync(t)
	fixtures, stdout, stderr := got.fixtures, got.stdout, got.stderr

	assert.Equal(t, 1, got.exitCode)
	assert.Equal(t, "quarry: warning: cannot delete snapshot "+fixtures[2].id+": permission denied; run quarry snapshots prune to try again\n"+
		interruptedWhilePruningLine, stderr)
	assert.Regexp(t, `(?m)^Store {5}`+regexp.QuoteMeta(storeShown)+`$`, stdout)
	assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 \(`+megabytes(middleBytes)+`\)\n$`, stdout)
	assert.NoFileExists(t, filepath.Join(got.dir, fixtures[1].id+".sqlite"))
	assert.FileExists(t, filepath.Join(got.dir, fixtures[0].id+".sqlite"))
}

func Test_run_sync_json_prints_the_document_when_interrupted_while_deleting_old_snapshots(t *testing.T) {
	got := interruptedSync(t, "--json")
	fixtures := got.fixtures

	assert.Equal(t, 1, got.exitCode)
	doc := decodeSyncDoc(t, got.stdout)
	require.NotNil(t, doc.Pruned)
	require.Len(t, doc.Pruned.Deleted, 1)
	assert.Equal(t, fixtures[1].id, doc.Pruned.Deleted[0].ID)
	require.Len(t, doc.Pruned.Failed, 1)
	assert.Equal(t, fixtures[2].id, doc.Pruned.Failed[0].ID)
	warning := "cannot delete snapshot " + fixtures[2].id + ": permission denied; run quarry snapshots prune to try again"
	assert.Equal(t, []string{warning}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+warning+"\n"+interruptedWhilePruningLine, got.stderr)
}

func Test_run_sync_succeeds_when_interrupted_with_no_snapshot_left_to_delete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := oldSnapshots(keptSnapshots)
	dir := writeSnapshots(t, home, fixtures...)
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	exitCode, stdout, stderr := runSyncRemoving(ctx, cancellingRemove(cancel, fixtures[0].id+".json", ""), "--quicken", bundle.Dir)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Regexp(t, `Pruned {4}1 snapshot beyond the newest 12 \(`+megabytes(oldestBytes)+`\)\n$`, stdout)
	assert.NoFileExists(t, filepath.Join(dir, fixtures[0].id+".sqlite"))
	assert.FileExists(t, orphan)
}
