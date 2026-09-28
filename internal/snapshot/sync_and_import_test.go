package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeImporter is a hand-written Importer fake: it records every path it
// was called with and returns the configured result, or err when set.
type fakeImporter struct {
	calls  []string
	result store.Result
	err    error
}

func (f *fakeImporter) Import(_ context.Context, snapshotPath string) (store.Result, error) {
	f.calls = append(f.calls, snapshotPath)
	if f.err != nil {
		return store.Result{}, f.err
	}
	return f.result, nil
}

// newImportServer builds a Server with a snapshots directory and a store
// path both under home, so refusal copy has something real to abbreviate.
func newImportServer(t *testing.T, home string, imp snapshot.Importer) *snapshot.Server {
	t.Helper()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	return snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
		snapshot.WithImporter(imp),
		snapshot.WithStorePath(filepath.Join(home, "quarry", "quarry.duckdb")),
	)
}

func snapshotIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".sqlite")
}

func Test_sync_and_import_imports_the_committed_snapshot(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{result: store.Result{
		Path: filepath.Join(home, "quarry", "quarry.duckdb"), Counts: store.Counts{Accounts: 2},
	}}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, outcome.Manifest.Snapshot.Path, fake.calls[0])
	require.NotNil(t, outcome.Store)
	assert.Equal(t, fake.result, *outcome.Store)
}

func Test_sync_and_import_skips_the_import_on_a_schema_mismatch(t *testing.T) {
	bundle := v9fixture.MissingSchemaBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

var errImportBoom = errors.New("boom from duckdb")

// The store refusal wraps the importer's own error (errors.Is) so a typed
// error such as *importer.UnmappableError still reaches a caller that asserts on it.
func Test_sync_and_import_frames_an_import_failure_as_a_store_refusal(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{err: errImportBoom}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errImportBoom)
	assert.Nil(t, outcome.Store)
	id := snapshotIDFromPath(outcome.Manifest.Snapshot.Path)
	want := fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s",
		homepath.Abbreviate(home, filepath.Join(home, "quarry")), errImportBoom.Error(), id)
	assert.Equal(t, want, err.Error())
	assert.FileExists(t, outcome.Manifest.Snapshot.Path)
	assert.FileExists(t, outcome.Manifest.Snapshot.Manifest)
}

func Test_sync_and_import_does_not_import_when_the_snapshot_fails(t *testing.T) {
	home := t.TempDir()
	fake := &fakeImporter{}
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithSource(&fakeSource{openErr: errBoom}),
		snapshot.WithImporter(fake),
		snapshot.WithStorePath(filepath.Join(home, "quarry", "quarry.duckdb")),
	)

	outcome, err := srv.SyncAndImport(t.Context(), filepath.Join(home, "Home.quicken"))

	require.Error(t, err)
	var mismatch snapshot.MismatchError
	assert.False(t, errors.As(err, &mismatch))
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

func Test_sync_and_import_refuses_without_an_importer(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	srv := newImportServer(t, home, nil)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	assert.Nil(t, outcome.Store)
}
