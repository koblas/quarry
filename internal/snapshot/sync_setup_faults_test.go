package snapshot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sync_fails_when_no_reference_is_configured(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(snapshot.WithSnapshotDir(t.TempDir()))

	_, err := srv.Sync(t.Context(), t.TempDir())

	require.Error(t, err)
}

// Write-safety guard: nothing reaches the snapshots directory before the
// probe succeeds.
func Test_sync_creates_nothing_when_the_probe_fails(t *testing.T) {
	t.Parallel()
	bundleDir := filepath.Join(t.TempDir(), "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))
	snapshotsDir := filepath.Join(t.TempDir(), "snapshots")
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
	)

	_, err := srv.Sync(t.Context(), bundleDir)

	require.Error(t, err)
	_, statErr := os.Stat(snapshotsDir)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_sync_wraps_an_error_when_opening_the_bundle_fails(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(t.TempDir()),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{openErr: errBoom}),
	)
	bundlePath := t.TempDir()

	_, err := srv.Sync(t.Context(), bundlePath)

	require.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "sync "+bundlePath)
}

func Test_sync_wraps_an_error_when_the_probe_fails(t *testing.T) {
	t.Parallel()
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(t.TempDir()),
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{probeErr: errBoom}),
	)
	bundlePath := t.TempDir()

	_, err := srv.Sync(t.Context(), bundlePath)

	require.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "sync "+bundlePath)
}
