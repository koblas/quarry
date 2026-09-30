package snapshot_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
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

func Test_import_from_puts_the_history_warning_after_the_one_sided_transfer_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry", "quarry.duckdb")}
	result := *builtWithOneSided(1)
	result.HistoryFault = fault
	srv := newImportServer(t, home, &fakeImporter{result: result})
	taken := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{
		"1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer",
		historyRestartLine("the file is not a DuckDB database"),
	}, outcome.Warnings())
}
