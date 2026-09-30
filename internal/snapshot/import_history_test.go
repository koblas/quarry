package snapshot_test

import (
	"errors"
	"path/filepath"
	"testing"

	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDriverText = errors.New("driver text")

func historyRestartLine(reason string) string {
	return "cannot carry import history forward from the previous store (" + reason + "); import_runs starts again with this sync"
}

func Test_sync_and_import_names_the_history_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	cases := []struct {
		name  string
		fault *store.OpenError
		want  string
	}{
		{
			"not a DuckDB database", &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath, Err: errDriverText},
			"the file is not a DuckDB database",
		},
		{
			"permission denied", &store.OpenError{Fault: store.OpenFaultPermission, Path: storePath, Err: errDriverText},
			"permission denied",
		},
		{
			"locked by another program", &store.OpenError{Fault: store.OpenFaultLocked, Path: storePath, Err: errDriverText},
			"another program has it open for writing",
		},
		{
			"repeated id", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table repeats an id"},
			"its import_runs table repeats an id",
		},
		{
			"no import_runs table", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "it has no import_runs table"},
			"it has no import_runs table",
		},
		{
			"incomplete import_runs table", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"},
			"its import_runs table is incomplete",
		},
		{
			"an id too large to follow", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table has an id too large to follow"},
			"its import_runs table has an id too large to follow",
		},
		{
			"other fault names the store by its display path", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "cannot open " + storePath + ": broken"},
			"cannot open ~/quarry/quarry.duckdb: broken",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeImporter{result: store.Result{Built: true, HistoryFault: c.fault}}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{historyRestartLine(c.want)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_warns_of_nothing_when_the_history_was_carried(t *testing.T) {
	t.Parallel()
	fake := &fakeImporter{result: store.Result{Built: true}}
	srv := newImportServer(t, t.TempDir(), fake)

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_import_from_puts_the_history_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry", "quarry.duckdb")}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: fault}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], historyRestartLine("the file is not a DuckDB database")}, outcome.Warnings())
}
