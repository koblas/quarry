package duckstore_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_built_from_names_the_latest_import_runs_snapshot(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	addImportRun(t, st, 3, "/snapshots/run-3.sqlite", 33)
	addImportRun(t, st, 2, "/snapshots/run-2.sqlite", 22)

	got, err := st.BuiltFrom(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "/snapshots/run-3.sqlite", got)
}

func Test_built_from_refuses_a_store_it_cannot_open_with_the_open_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		st   func(t *testing.T) *duckstore.Store
		want store.OpenFault
	}{
		{name: "no file", st: func(t *testing.T) *duckstore.Store {
			t.Helper()
			return duckstore.New(t.TempDir())
		}, want: store.OpenFaultMissing},
		{
			name: "a file that is not a database",
			st: func(t *testing.T) *duckstore.Store {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
				return duckstore.New(dir)
			},
			want: store.OpenFaultNotDuckDB,
		},
		{
			name: "a file the process may not read",
			st: func(t *testing.T) *duckstore.Store {
				t.Helper()
				skipAsRoot(t)
				dir := t.TempDir()
				path := filepath.Join(dir, duckstore.FileName)
				require.NoError(t, os.WriteFile(path, []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))
				return duckstore.New(dir)
			},
			want: store.OpenFaultPermission,
		},
		{
			name: "another process holds the file",
			st: func(t *testing.T) *duckstore.Store {
				t.Helper()
				return newBuiltStore(t, failingOpener(driverIOError(`IO Error: Could not set lock on file "x.duckdb": Conflicting lock is held in `+
					`/usr/local/bin/quarry (PID 42) by user dave. See also https://duckdb.org/docs/stable/connect/concurrency`)))
			},
			want: store.OpenFaultLocked,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := c.st(t)

			got, err := st.BuiltFrom(t.Context())

			openErr, ok := errors.AsType[*store.OpenError](err)
			require.True(t, ok, "want *store.OpenError, got %v", err)
			assert.Equal(t, c.want, openErr.Fault)
			assert.Empty(t, got)
		})
	}
}

func Test_built_from_refuses_a_store_of_another_format_with_the_snapshot_it_names(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsDDL)

	got, err := st.BuiltFrom(t.Context())

	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultOtherFormat, openErr.Fault)
	assert.Equal(t, snapshotFile, openErr.SnapshotPath)
	assert.Empty(t, got)
}

func Test_built_from_refuses_a_store_without_an_import_run(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.ImportRuns = nil
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	_, err = st.BuiltFrom(t.Context())

	assertOtherFault(t, err, "the store has no import history")
}

func Test_built_from_returns_the_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	spy := &spyReadDB{checkFaults: map[string]error{duckstore.SnapshotPathQuery: fault}}
	st := newBuiltStore(t, spyOpener(spy))

	_, err := st.BuiltFrom(t.Context())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_built_from_closes_the_connection_it_read_through(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{}
	st := newBuiltStore(t, spyOpener(spy))

	_, err := st.BuiltFrom(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 1, spy.closes)
}
