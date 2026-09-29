package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{name: "backup fails with a classified sqlite fault", dir: t.TempDir(),
			src: &fakeSource{backupErr: sqlite3.Error{Code: sqlite3.ErrBusy}}},
		{name: "backup fails with an unclassified cause", dir: t.TempDir(), src: &fakeSource{backupErr: errBoom}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", re.Error())
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

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", re.Error())
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
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&cancelAndFailWriteManifestDestination{
			real:   snapshot.NewDirDestination(snapshotsDir),
			cancel: cancel,
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(ctx, bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", re.Error())
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
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&cancelAfterWriteManifestDestination{
			real:   snapshot.NewDirDestination(snapshotsDir),
			cancel: cancel,
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(ctx, bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "sync interrupted; nothing was kept; run quarry sync again", re.Error())
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
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&cancelDuringCommitManifestDestination{
			real:   snapshot.NewDirDestination(snapshotsDir),
			cancel: cancel,
		}),
		snapshot.WithHome(home),
	)

	manifest, err := srv.Sync(ctx, bundle.Dir)

	require.NoError(t, err)
	assert.True(t, manifest.Schema.Verified)
	assert.FileExists(t, manifest.Snapshot.Path)
	assert.FileExists(t, manifest.Snapshot.Manifest)
}
