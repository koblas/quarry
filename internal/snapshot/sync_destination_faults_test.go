package snapshot_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A write failure that is neither a permission error nor disk-full/over-quota
// gets its own text, distinct from the disk-full wording.
func Test_sync_refuses_when_writing_the_manifest_fails_with_an_unclassified_cause(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:              snapshot.NewDirDestination(snapshotsDir),
			failWriteManifest: &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.EIO},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: input/output error; run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

// mkdirAllCause reproduces os.MkdirAll's own failure against an existing
// blockedPath through a connection independent of the code under test, so a
// test can pin the exact OS reason rather than asserting only "some error".
func mkdirAllCause(t *testing.T, blockedPath string) string {
	t.Helper()
	err := os.MkdirAll(blockedPath, 0o700)
	require.Error(t, err)
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
	return pathErr.Err.Error()
}

// Exercises the real Destination adapter's Prepare fault path: MkdirAll
// fails because the configured snapshots path is already a regular file.
func Test_sync_refuses_when_the_snapshots_directory_cannot_be_prepared(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	home := t.TempDir()
	blockedPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))
	cause := mkdirAllCause(t, blockedPath)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(blockedPath),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write to "+blockedPath+": "+cause+"; make the directory writable by your user",
		re.Error())
}

// partialFaultDestination wraps the real production Destination and injects
// exactly one failing method, so the rest of Sync's pipeline (a real
// backup and schema read) runs for real.
type partialFaultDestination struct {
	real                                              snapshot.Destination
	failWriteManifest, failCommitManifest, failCommit error
	failDiscard                                       error

	backedUpPartial, discardedPartial string
}

func (f *partialFaultDestination) Prepare(ctx context.Context) error { return f.real.Prepare(ctx) }
func (f *partialFaultDestination) Backup(ctx context.Context, src snapshot.Source, name string) (string, string, error) {
	partial, resolvedName, err := f.real.Backup(ctx, src, name)
	f.backedUpPartial = partial
	return partial, resolvedName, err
}
func (f *partialFaultDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	if f.failWriteManifest != nil {
		return "", f.failWriteManifest
	}
	return f.real.WriteManifest(ctx, name, data)
}
func (f *partialFaultDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	if f.failCommitManifest != nil {
		return "", f.failCommitManifest
	}
	return f.real.CommitManifest(ctx, partial)
}
func (f *partialFaultDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	if f.failCommit != nil {
		return "", f.failCommit
	}
	return f.real.CommitSnapshot(ctx, partial)
}
func (f *partialFaultDestination) FinalPaths(name string) (string, string) {
	return f.real.FinalPaths(name)
}
func (f *partialFaultDestination) Discard(ctx context.Context, partial string) error {
	f.discardedPartial = partial
	err := f.real.Discard(ctx, partial)
	if f.failDiscard != nil {
		return f.failDiscard
	}
	return err
}

func Test_sync_refuses_when_writing_the_manifest_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:              snapshot.NewDirDestination(snapshotsDir),
			failWriteManifest: &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

func Test_sync_refuses_when_committing_the_manifest_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:               snapshot.NewDirDestination(snapshotsDir),
			failCommitManifest: &os.LinkError{Op: "link", Old: "manifest.json.partial", New: "manifest.json", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

// The manifest final is already committed when CommitSnapshot fails; the
// empty directory afterward proves Discard removed that final too.
func Test_sync_refuses_when_committing_the_snapshot_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsDir := filepath.Join(home, "snapshots")
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(&partialFaultDestination{
			real:       snapshot.NewDirDestination(snapshotsDir),
			failCommit: &os.LinkError{Op: "link", Old: "snapshot.sqlite.partial", New: "snapshot.sqlite", Err: syscall.ENOSPC},
		}),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
		re.Error())
	assertSnapshotsDirEmpty(t, snapshotsDir)
}

func Test_sync_still_returns_the_classified_refusal_when_discard_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	dest := &partialFaultDestination{
		real:        snapshot.NewDirDestination(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")),
		failDiscard: errBoom,
	}
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithDestination(dest),
		snapshot.WithHome(home),
	)

	_, err = srv.Sync(t.Context(), bundle.Dir)

	var re snapshot.RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t,
		"~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>",
		re.Error())
	assert.NotEmpty(t, dest.backedUpPartial)
	assert.Equal(t, dest.backedUpPartial, dest.discardedPartial)
}

// A Discard failure is best-effort: it never replaces the write refusal
// already classified for the write failure that triggered it.
func Test_sync_still_returns_the_write_refusal_when_discard_fails_after_a_write_failure(t *testing.T) {
	t.Parallel()
	writeErr := &fs.PathError{Op: "open", Path: "manifest.json.partial", Err: syscall.ENOSPC}

	cases := []struct {
		name               string
		failWriteManifest  error
		failCommitManifest error
		failCommit         error
	}{
		{name: "write manifest fails", failWriteManifest: writeErr},
		{name: "commit manifest fails", failCommitManifest: writeErr},
		{name: "commit snapshot fails", failCommit: writeErr},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			snapshotsDir := filepath.Join(home, "snapshots")
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			ref, err := v9.Reference(t.Context())
			require.NoError(t, err)
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(snapshotsDir),
				snapshot.WithReference(v9.ReferenceLabel, ref),
				snapshot.WithDestination(&partialFaultDestination{
					real:               snapshot.NewDirDestination(snapshotsDir),
					failWriteManifest:  c.failWriteManifest,
					failCommitManifest: c.failCommitManifest,
					failCommit:         c.failCommit,
					failDiscard:        errBoom,
				}),
				snapshot.WithHome(home),
			)

			_, err = srv.Sync(t.Context(), bundle.Dir)

			var re snapshot.RefusalError
			require.ErrorAs(t, err, &re)
			assert.Equal(t,
				"cannot write snapshot to ~/snapshots: no space left on device; free disk space, then run quarry sync again",
				re.Error())
			// failDiscard only overrides the returned error; each call still removes its own real file.
			assertSnapshotsDirEmpty(t, snapshotsDir)
		})
	}
}
