package snapshot_test

import (
	"context"
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
	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sync_writes_a_verified_private_snapshot_of_an_open_file(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref := v9Reference(t)
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
	assert.Contains(t, mismatch.Error(), "is missing 1 table that the schema reference expects")
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
}

func Test_sync_reports_a_missing_column_when_the_reference_names_one_the_bundle_lacks(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref := v9Reference(t)
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
	assert.Contains(t, mismatch.Error(), "is missing 1 column that the schema reference expects")
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
}

func Test_sync_stays_verified_and_lists_unexpected_tables_when_the_bundle_has_extra_tables_only(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref := v9Reference(t)
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

func Test_sync_warns_with_correct_singular_plural_agreement_for_extras(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		dropTables   []string
		dropColumns  int
		wantFragment string
	}{
		{
			name: "one extra table only", dropTables: []string{"ZALERT"},
			wantFragment: "has 1 table that is not in the schema reference; quarry ignores it",
		},
		{
			name: "one extra column only", dropColumns: 1,
			wantFragment: "has 1 column that is not in the schema reference; quarry ignores it",
		},
		{
			name: "many extra columns only", dropColumns: 2,
			wantFragment: "has 2 columns that are not in the schema reference; quarry ignores them",
		},
		{
			name: "one extra table and one extra column", dropTables: []string{"ZALERT"}, dropColumns: 1,
			wantFragment: "has 1 table and 1 column that are not in the schema reference; quarry ignores them",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			ref := v9Reference(t)
			for _, table := range c.dropTables {
				delete(ref, table)
			}
			if c.dropColumns > 0 {
				cols := ref["ZACCOUNT"]
				ref["ZACCOUNT"] = cols[:len(cols)-c.dropColumns]
			}
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(filepath.Join(t.TempDir(), "snapshots")),
				snapshot.WithReference(v9.ReferenceLabel, ref),
			)

			manifest, err := srv.Sync(t.Context(), bundle.Dir)

			require.NoError(t, err)
			require.Len(t, manifest.Warnings, 1)
			assert.Contains(t, manifest.Warnings[0], c.wantFragment)
		})
	}
}

func Test_sync_does_not_populate_warnings_when_extras_are_accompanied_by_missing_entries(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref := v9Reference(t)
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
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.Equal(t, 82, manifest.Schema.ReferenceTables)
	assert.Equal(t, 1838, manifest.Schema.ReferenceColumns)

	encoded, err := manifest.Encode()
	require.NoError(t, err)
	bare := manifest
	bare.Schema.ReferenceTables, bare.Schema.ReferenceColumns = 0, 0
	bareEncoded, err := bare.Encode()
	require.NoError(t, err)
	assert.Equal(t, string(bareEncoded), string(encoded))
}

func Test_sync_writes_a_manifest_whose_own_path_fields_match_where_it_is_committed(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := newServer(t, snapshotsDir)

	before, err := os.ReadFile(bundle.DataPath)
	require.NoError(t, err)
	statBefore, err := os.Stat(bundle.DataPath)
	require.NoError(t, err)
	namesBefore := dirNames(t, bundle.Dir)

	manifest, err := srv.Sync(t.Context(), bundle.Dir)
	require.NoError(t, err)
	require.FileExists(t, manifest.Snapshot.Path)
	require.FileExists(t, manifest.Snapshot.Manifest)

	after, err := os.ReadFile(bundle.DataPath)
	require.NoError(t, err)
	statAfter, err := os.Stat(bundle.DataPath)
	require.NoError(t, err)
	namesAfter := dirNames(t, bundle.Dir)

	assert.Equal(t, before, after)
	assert.True(t, statBefore.ModTime().Equal(statAfter.ModTime()))
	assert.Equal(t, namesBefore, namesAfter)
}

func Test_sync_appends_a_suffix_when_the_current_second_already_has_a_snapshot(t *testing.T) {
	t.Parallel()
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

	// Located by directory listing, not by the returned Manifest.
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

// A cancelled ctx overrides whatever refusal each pre-commit failure site
// would otherwise classify to, across every failure kind.
func Test_sync_reports_interrupted_when_the_context_is_already_cancelled_at_a_precommit_failure(t *testing.T) {
	t.Parallel()
	blockedPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))

	cases := []struct {
		name string
		dir  string
		src  *fakeSource
	}{
		{name: "open fails", dir: t.TempDir(), src: &fakeSource{openErr: errBoom}},
		{name: "probe fails", dir: t.TempDir(), src: &fakeSource{probeErr: errBoom}},
		{name: "prepare fails", dir: blockedPath, src: &fakeSource{}},
		{
			name: "backup fails with a classified sqlite fault", dir: t.TempDir(),
			src: &fakeSource{backupErr: sqlite3.Error{Code: sqlite3.ErrBusy}},
		},
		{name: "backup fails with an unclassified cause", dir: t.TempDir(), src: &fakeSource{backupErr: errBoom}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(c.dir),
				snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
				snapshot.WithSource(c.src),
				snapshot.WithHome(home),
			)

			_, err := srv.Sync(ctx, filepath.Join(home, "Documents", "Home.quicken"))

			assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", refusalText(t, err))
		})
	}
}

// buildManifest's own ctx-cancellation failure (opening the snapshot copy)
// is also routed through FailureOutcome, distinct from the sites above.
func Test_sync_reports_interrupted_when_the_context_is_already_cancelled_during_buildManifest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: filepath.Join(t.TempDir(), "missing.sqlite")}),
	)

	_, err := srv.Sync(ctx, t.TempDir())

	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", refusalText(t, err))
}

// cancelAndFailWriteManifestDestination cancels ctx and fails WriteManifest
// in the same call, landing exactly on that failure site's FailureOutcome check.
type cancelAndFailWriteManifestDestination struct {
	real   snapshot.Destination
	cancel context.CancelFunc
}

func (f *cancelAndFailWriteManifestDestination) Prepare(ctx context.Context) error {
	return f.real.Prepare(ctx)
}

func (f *cancelAndFailWriteManifestDestination) Backup(ctx context.Context, src snapshot.Source, name string) (string, string, error) {
	return f.real.Backup(ctx, src, name)
}

func (f *cancelAndFailWriteManifestDestination) WriteManifest(context.Context, string, []byte) (string, error) {
	f.cancel()
	return "", errBoom
}

func (f *cancelAndFailWriteManifestDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	return f.real.CommitManifest(ctx, partial)
}

func (f *cancelAndFailWriteManifestDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	return f.real.CommitSnapshot(ctx, partial)
}

func (f *cancelAndFailWriteManifestDestination) FinalPaths(name string) (string, string) {
	return f.real.FinalPaths(name)
}

func (f *cancelAndFailWriteManifestDestination) Discard(ctx context.Context, partial string) error {
	return f.real.Discard(ctx, partial)
}

func Test_sync_reports_interrupted_when_the_context_ends_exactly_when_writing_the_manifest_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	srv := newDestinationServer(t, home, snapshotsDir, &cancelAndFailWriteManifestDestination{
		real:   snapshot.NewDirDestination(snapshotsDir),
		cancel: cancel,
	})

	_, err := srv.Sync(ctx, bundle.Dir)

	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", refusalText(t, err))
}

// cancelAfterWriteManifestDestination wraps the real adapter and cancels ctx
// once the manifest partial is actually on disk, so a test can land exactly
// on Sync's single pre-commit ctx check.
type cancelAfterWriteManifestDestination struct {
	real   snapshot.Destination
	cancel context.CancelFunc
}

func (f *cancelAfterWriteManifestDestination) Prepare(ctx context.Context) error {
	return f.real.Prepare(ctx)
}

func (f *cancelAfterWriteManifestDestination) Backup(ctx context.Context, src snapshot.Source, name string) (string, string, error) {
	return f.real.Backup(ctx, src, name)
}

func (f *cancelAfterWriteManifestDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	partial, err := f.real.WriteManifest(ctx, name, data)
	if err == nil {
		f.cancel()
	}
	return partial, err
}

func (f *cancelAfterWriteManifestDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	return f.real.CommitManifest(ctx, partial)
}

func (f *cancelAfterWriteManifestDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	return f.real.CommitSnapshot(ctx, partial)
}

func (f *cancelAfterWriteManifestDestination) FinalPaths(name string) (string, string) {
	return f.real.FinalPaths(name)
}

func (f *cancelAfterWriteManifestDestination) Discard(ctx context.Context, partial string) error {
	return f.real.Discard(ctx, partial)
}

func Test_sync_discards_everything_and_reports_interrupted_when_the_context_ends_just_before_the_commit_sequence(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	srv := newDestinationServer(t, home, snapshotsDir, &cancelAfterWriteManifestDestination{
		real:   snapshot.NewDirDestination(snapshotsDir),
		cancel: cancel,
	})

	_, err := srv.Sync(ctx, bundle.Dir)

	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", refusalText(t, err))
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

// cancelDuringCommitManifestDestination cancels ctx from inside
// CommitManifest itself, after the pre-commit checkpoint has already passed.
type cancelDuringCommitManifestDestination struct {
	real   snapshot.Destination
	cancel context.CancelFunc
}

func (f *cancelDuringCommitManifestDestination) Prepare(ctx context.Context) error {
	return f.real.Prepare(ctx)
}

func (f *cancelDuringCommitManifestDestination) Backup(ctx context.Context, src snapshot.Source, name string) (string, string, error) {
	return f.real.Backup(ctx, src, name)
}

func (f *cancelDuringCommitManifestDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	return f.real.WriteManifest(ctx, name, data)
}

func (f *cancelDuringCommitManifestDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	f.cancel()
	return f.real.CommitManifest(ctx, partial)
}

func (f *cancelDuringCommitManifestDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	return f.real.CommitSnapshot(ctx, partial)
}

func (f *cancelDuringCommitManifestDestination) FinalPaths(name string) (string, string) {
	return f.real.FinalPaths(name)
}

func (f *cancelDuringCommitManifestDestination) Discard(ctx context.Context, partial string) error {
	return f.real.Discard(ctx, partial)
}

// Once the pre-commit checkpoint has passed, ctx ending mid-rename must not
// abort the sequence.
func Test_sync_completes_normally_when_the_context_ends_during_the_commit_sequence(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	srv := newDestinationServer(t, home, snapshotsDir, &cancelDuringCommitManifestDestination{
		real:   snapshot.NewDirDestination(snapshotsDir),
		cancel: cancel,
	})

	manifest, err := srv.Sync(ctx, bundle.Dir)

	require.NoError(t, err)
	assert.True(t, manifest.Schema.Verified)
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
}
