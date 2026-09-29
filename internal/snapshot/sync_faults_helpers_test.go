package snapshot_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource is a hand-written Source fake: each call returns the
// configured error, or succeeds when it is nil.
type fakeSource struct {
	openErr, probeErr, backupErr error
}

func (f *fakeSource) Open(context.Context, string) error   { return f.openErr }
func (f *fakeSource) Probe(context.Context) error          { return f.probeErr }
func (f *fakeSource) Backup(context.Context, string) error { return f.backupErr }
func (f *fakeSource) Close() error                         { return nil }

var errBoom = errors.New("boom")

// fixedPathDestination hands Backup's caller a pre-built file instead of
// really backing anything up, so a test controls exactly what buildManifest
// reads.
type fixedPathDestination struct {
	snapshotPath string
}

func (f *fixedPathDestination) Prepare(context.Context) error { return nil }
func (f *fixedPathDestination) Backup(_ context.Context, _ snapshot.Source, name string) (string, string, error) {
	return f.snapshotPath, name, nil
}

func (f *fixedPathDestination) WriteManifest(context.Context, string, []byte) (string, error) {
	return "", nil
}

func (f *fixedPathDestination) CommitSnapshot(context.Context, string) (string, error) {
	return "", nil
}

func (f *fixedPathDestination) CommitManifest(context.Context, string) (string, error) {
	return "", nil
}
func (f *fixedPathDestination) FinalPaths(string) (string, string)    { return "", "" }
func (f *fixedPathDestination) Discard(context.Context, string) error { return nil }

// assertSnapshotsDirEmpty fails if dir holds anything at all: unlike
// assertNoPartialsLeftBehind, it also catches a committed final Discard
// should have removed.
func assertSnapshotsDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
