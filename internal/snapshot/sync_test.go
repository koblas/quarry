package snapshot_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newServer(t *testing.T, snapshotsDir string) *snapshot.Server {
	t.Helper()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	return snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	)
}

func Test_sync_writes_a_verified_private_snapshot_of_an_open_file(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "Application Support", "quarry", "snapshots")
	srv := newServer(t, snapshotsDir)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assertMode(t, snapshotsDir, 0o700)
	assertMode(t, manifest.Snapshot.Path, 0o600)
	assertMode(t, manifest.Snapshot.Manifest, 0o600)
	assert.Equal(t, 2, manifest.Snapshot.Accounts)
	assert.True(t, manifest.Schema.Verified)
	assert.Equal(t, manifest.Schema.ReferenceFingerprint, manifest.Schema.Fingerprint)

	raw, err := os.ReadFile(manifest.Snapshot.Path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	assert.Equal(t, hex.EncodeToString(sum[:]), manifest.Snapshot.SHA256)

	snap, err := sqlite.OpenReadOnly(t.Context(), manifest.Snapshot.Path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = snap.Close() })
	count, err := snap.QueryInt(t.Context(), "SELECT count(*) FROM ZACCOUNT WHERE ZNAME = ?", bundle.WALOnlyAccount)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func Test_sync_reports_verified_false_when_the_reference_names_a_table_the_bundle_lacks(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ref["ZFAKETABLE"] = []string{"ZFAKECOLUMN"}
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(t.TempDir(), "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.False(t, manifest.Schema.Verified)
	assert.Equal(t, []string{"ZFAKETABLE"}, manifest.Schema.MissingTables)
	assert.NotEqual(t, manifest.Schema.ReferenceFingerprint, manifest.Schema.Fingerprint)
}

func Test_sync_reports_a_missing_column_when_the_reference_names_one_the_bundle_lacks(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ref["ZACCOUNT"] = append(ref["ZACCOUNT"], "ZFAKECOLUMN")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(t.TempDir(), "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.False(t, manifest.Schema.Verified)
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZFAKECOLUMN"}}, manifest.Schema.MissingColumns)
}

func Test_sync_reports_a_warning_when_the_bundle_has_extra_tables_only(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	delete(ref, "ZALERT")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(t.TempDir(), "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.True(t, manifest.Schema.Verified)
	assert.Equal(t, []string{"ZALERT"}, manifest.Schema.UnexpectedTables)
	assert.Empty(t, manifest.Schema.MissingTables)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm())
}

func Test_sync_leaves_the_live_bundle_unchanged(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	before, err := os.ReadFile(bundle.DataPath)
	require.NoError(t, err)
	statBefore, err := os.Stat(bundle.DataPath)
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)

	_, err = srv.Sync(t.Context(), bundle.Dir)
	require.NoError(t, err)

	after, err := os.ReadFile(bundle.DataPath)
	require.NoError(t, err)
	statAfter, err := os.Stat(bundle.DataPath)
	require.NoError(t, err)
	entriesAfter, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)

	assert.Equal(t, before, after)
	assert.True(t, statBefore.ModTime().Equal(statAfter.ModTime()))
	assert.Equal(t, namesOf(entriesBefore), namesOf(entriesAfter))
}

func namesOf(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}
