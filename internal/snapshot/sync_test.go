package snapshot_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.False(t, manifest.Schema.Verified)
	assert.Equal(t, []string{"ZFAKETABLE"}, manifest.Schema.MissingTables)
	assert.NotEqual(t, manifest.Schema.ReferenceFingerprint, manifest.Schema.Fingerprint)
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
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

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.False(t, manifest.Schema.Verified)
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZFAKECOLUMN"}}, manifest.Schema.MissingColumns)
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
}

func Test_sync_stays_verified_and_lists_unexpected_tables_when_the_bundle_has_extra_tables_only(t *testing.T) {
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
	require.Len(t, manifest.Warnings, 1)
	assert.Contains(t, manifest.Warnings[0], "1 table")
	assert.Contains(t, manifest.Warnings[0], "not in the schema reference")
}

func Test_sync_does_not_populate_warnings_when_extras_are_accompanied_by_missing_entries(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	delete(ref, "ZALERT")
	ref["ZFAKETABLE"] = []string{"ZFAKECOLUMN"}
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(t.TempDir(), "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.False(t, manifest.Schema.Verified)
	assert.NotEmpty(t, manifest.Schema.UnexpectedTables)
	assert.Empty(t, manifest.Warnings)
}

// Pinned against Test_scope_of_the_reference_has_the_pinned_table_and_column_counts.
func Test_sync_reports_the_scoped_reference_table_and_column_counts(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.Equal(t, 82, manifest.Schema.ReferenceTables)
	assert.Equal(t, 1835, manifest.Schema.ReferenceColumns)

	encoded, err := manifest.Encode()
	require.NoError(t, err)
	bare := manifest
	bare.Schema.ReferenceTables, bare.Schema.ReferenceColumns = 0, 0
	bareEncoded, err := bare.Encode()
	require.NoError(t, err)
	assert.Equal(t, string(bareEncoded), string(encoded))
}

func Test_sync_writes_a_manifest_whose_own_path_fields_match_where_it_is_committed(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	raw, err := os.ReadFile(manifest.Snapshot.Manifest)
	require.NoError(t, err)
	var onDisk snapshot.Manifest
	require.NoError(t, json.Unmarshal(raw, &onDisk))
	assert.Equal(t, manifest.Snapshot.Path, onDisk.Snapshot.Path)
	assert.Equal(t, manifest.Snapshot.Manifest, onDisk.Snapshot.Manifest)
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

	manifest, err := srv.Sync(t.Context(), bundle.Dir)
	require.NoError(t, err)
	require.FileExists(t, manifest.Snapshot.Path)
	require.FileExists(t, manifest.Snapshot.Manifest)

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

func Test_sync_appends_a_suffix_when_the_current_second_already_has_a_snapshot(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	var first snapshot.Manifest
	var firstSQLiteBefore, firstManifestBefore []byte
	var firstModTimeBefore time.Time

	// synctest freezes time.Now(), so both calls land in the same second.
	synctest.Test(t, func(t *testing.T) {
		var err error
		first, err = srv.Sync(t.Context(), bundle.Dir)
		require.NoError(t, err)

		firstSQLiteBefore, err = os.ReadFile(first.Snapshot.Path)
		require.NoError(t, err)
		firstManifestBefore, err = os.ReadFile(first.Snapshot.Manifest)
		require.NoError(t, err)
		info, err := os.Stat(first.Snapshot.Path)
		require.NoError(t, err)
		firstModTimeBefore = info.ModTime()

		_, err = srv.Sync(t.Context(), bundle.Dir)
		require.NoError(t, err)
	})

	firstSQLiteAfter, err := os.ReadFile(first.Snapshot.Path)
	require.NoError(t, err)
	assert.Equal(t, firstSQLiteBefore, firstSQLiteAfter)
	firstManifestAfter, err := os.ReadFile(first.Snapshot.Manifest)
	require.NoError(t, err)
	assert.Equal(t, firstManifestBefore, firstManifestAfter)
	infoAfter, err := os.Stat(first.Snapshot.Path)
	require.NoError(t, err)
	assert.True(t, firstModTimeBefore.Equal(infoAfter.ModTime()))

	// Located by directory listing, not by the returned Manifest, so a
	// wrong-but-self-consistent returned path cannot make this pass.
	secondManifestPath := onlyFileWithSuffix(t, snapshotsDir, "_2.json")
	secondSQLitePath := onlyFileWithSuffix(t, snapshotsDir, "_2.sqlite")
	assert.FileExists(t, secondSQLitePath)
	raw, err := os.ReadFile(secondManifestPath)
	require.NoError(t, err)
	var onDisk snapshot.Manifest
	require.NoError(t, json.Unmarshal(raw, &onDisk))
	assert.True(t, strings.HasSuffix(onDisk.Snapshot.Path, "_2.sqlite"), "got %s", onDisk.Snapshot.Path)
	assert.True(t, strings.HasSuffix(onDisk.Snapshot.Manifest, "_2.json"), "got %s", onDisk.Snapshot.Manifest)
}

// onlyFileWithSuffix fails the test unless exactly one entry in dir ends in
// suffix, returning its full path.
func onlyFileWithSuffix(t *testing.T, dir, suffix string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = append(found, e.Name())
		}
	}
	require.Len(t, found, 1, "expected exactly one %s file in %s, found %v", suffix, dir, found)
	return filepath.Join(dir, found[0])
}

func namesOf(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}
