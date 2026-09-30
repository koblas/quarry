package snapshot_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRemover records the base name of each file Prune removes, in order, and fails those
// named in faults; every other file is really removed.
type fakeRemover struct {
	calls  []string
	faults map[string]error
	// cancel, when set, ends the context once cancelAfter has been removed (or has failed).
	cancelAfter string
	cancel      context.CancelFunc
}

func (r *fakeRemover) remove(path string) error {
	name := filepath.Base(path)
	r.calls = append(r.calls, name)
	if r.cancel != nil && name == r.cancelAfter {
		defer r.cancel()
	}
	if err, ok := r.faults[name]; ok {
		return err
	}
	return os.Remove(path)
}

// failing is a fakeRemover that fails name with the *fs.PathError os.Remove returns for errno.
func failing(name string, errno syscall.Errno) *fakeRemover {
	return &fakeRemover{faults: map[string]error{name: &fs.PathError{Op: "remove", Path: name, Err: errno}}}
}

// newPruneServer builds a Server over home's snapshots folder whose removes go through rm.
func newPruneServer(home string, probe snapshot.StoreProbe, rm *fakeRemover) *snapshot.Server {
	opts := []snapshot.Option{
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")), snapshot.WithHome(home), snapshot.WithRemove(rm.remove),
	}
	if probe != nil {
		opts = append(opts, snapshot.WithStoreProbe(probe))
	}
	return snapshot.NewServer(opts...)
}

// prunable writes a snapshot with a manifest for each id under home's folder and returns the folder.
func prunable(t *testing.T, home string, ids ...string) string {
	t.Helper()
	dir := snapshotsFolder(t, home)
	for _, id := range ids {
		writeSnapshot(t, dir, id, 1000)
		writeManifest(t, dir, id, manifestTaken("2026-09-27T10:00:00Z"))
	}
	return dir
}

func Test_prune_deletes_the_snapshots_beyond_the_newest_n_with_their_manifests(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest, idFourth}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Equal(t, filepath.Join(home, "snapshots"), pruned.Dir)
	for _, id := range []string{idOldest, idFourth} {
		assert.NoFileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.NoFileExists(t, filepath.Join(dir, id+".json"))
	}
	for _, id := range []string{idNewest, idMiddle} {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
}

func Test_prune_deletes_the_snapshot_file_then_its_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest + ".sqlite", idOldest + ".json"}, rm.calls)
}

func Test_prune_leaves_the_manifest_when_the_snapshot_file_cannot_be_deleted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	rm := failing(idOldest+".sqlite", syscall.EACCES)

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.NotContains(t, rm.calls, idOldest+".json")
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_reports_a_snapshot_it_could_not_delete_and_goes_on_to_the_next(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".sqlite", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, idOldest, pruned.Failed[0].Entry.ID)
	assert.Equal(t, "permission denied", pruned.Failed[0].Reason)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 3, pruned.Snapshots)
}

func Test_prune_deletes_a_snapshot_that_has_no_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	for _, id := range []string{idNewest, idMiddle, idOldest} {
		writeSnapshot(t, dir, id, 1000)
	}

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
}

func Test_prune_counts_a_snapshot_deleted_when_only_its_manifest_cannot_be_removed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".json", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_drops_a_snapshot_file_that_is_already_gone_and_removes_its_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".sqlite", syscall.ENOENT)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
}
