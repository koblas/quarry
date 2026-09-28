package duckstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_replace_removes_a_leftover_partial_just_past_the_age_gate(t *testing.T) {
	dir := t.TempDir()
	leftover := filepath.Join(dir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
	old := time.Now().Add(-61 * time.Minute)
	require.NoError(t, os.Chtimes(leftover, old, old))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	_, statErr := os.Stat(leftover)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_replace_leaves_a_leftover_partial_just_inside_the_age_gate_alone(t *testing.T) {
	dir := t.TempDir()
	leftover := filepath.Join(dir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
	recent := time.Now().Add(-59 * time.Minute)
	require.NoError(t, os.Chtimes(leftover, recent, recent))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	_, statErr := os.Stat(leftover)
	assert.NoError(t, statErr)
}

// Each case is old enough for the age gate alone to remove it, so only the
// pattern's precision keeps it.
func Test_replace_sweep_leaves_near_miss_and_unrelated_files_alone(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{"no .partial suffix", ".quarry-20260927T143005Z.duckdb"},
		{"no leading dot", "quarry-20260927T143005Z.duckdb.partial"},
		{"trailing suffix after partial", ".quarry-20260927T143005Z.duckdb.partial.bak"},
		{"extra prefix before the dot", "x.quarry-20260927T143005Z.duckdb.partial"},
		{"sqlite wal suffix, not duckdb's", ".quarry-20260927T143005Z.duckdb.partial-wal"},
		{"snapshot-shaped, wrong extension", ".20260927T143005Z.sqlite.partial"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, c.file)
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
			old := time.Now().Add(-2 * time.Hour)
			require.NoError(t, os.Chtimes(path, old, old))
			st := duckstore.New(dir)

			_, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			_, statErr := os.Stat(path)
			assert.NoError(t, statErr)
		})
	}
}

func Test_replace_sweep_ignores_subdirectories(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "snapshots"), 0o700))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	info, statErr := os.Stat(filepath.Join(dir, "snapshots"))
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// A concurrent sync's own in-flight partial matches the pattern but is not
// yet aged; the sweep must not race it.
func Test_replace_leaves_a_fresh_partial_alone(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, ".quarry-20260927T143005Z.duckdb.partial")
	require.NoError(t, os.WriteFile(fresh, []byte("in flight"), 0o600))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	_, statErr := os.Stat(fresh)
	assert.NoError(t, statErr)
}

// s.dir is a regular file, so the sweep's own os.ReadDir fails; the build
// attempt still runs and fails on its own account.
func Test_replace_ignores_a_sweep_that_cannot_list_the_directory(t *testing.T) {
	parent := t.TempDir()
	notADir := filepath.Join(parent, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))
	st := duckstore.New(notADir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.Error(t, err)
}

// dir is chmoded read-only so the sweep's os.Remove fails on the leftover;
// WithCreate fails independently of the filesystem, proving the sweep's own
// fault never surfaces through Replace's return.
func Test_replace_leaves_a_leftover_alone_when_the_sweep_cannot_remove_it(t *testing.T) {
	dir := t.TempDir()
	leftover := filepath.Join(dir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(leftover, old, old))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	st := duckstore.New(dir, duckstore.WithCreate(func(context.Context, string) (duckstore.DB, error) {
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	_, statErr := os.Stat(leftover)
	assert.NoError(t, statErr)
}
