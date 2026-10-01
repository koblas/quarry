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

func findingsRestartLine(reason string) string {
	return "cannot carry findings forward from the previous store (" + reason + "); findings history starts again with this sync"
}

func combinedCarryLine(reason string) string {
	return "cannot carry import history, findings or exchange rates forward from the previous store (" + reason + "); all three start again with this sync"
}

func ratesRestartLine(reason string) string {
	return "cannot carry exchange rates forward from the previous store (" + reason + "); fetching them all again"
}

func Test_sync_and_import_names_the_combined_reason_when_the_previous_store_cannot_be_read(t *testing.T) {
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
			"other fault names the store by its display path", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "cannot open " + storePath + ": broken"},
			"cannot open ~/quarry/quarry.duckdb: broken",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeImporter{result: store.Result{Built: true, HistoryFault: c.fault, StoreUnreadable: true}}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{combinedCarryLine(c.want)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_names_the_history_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{
		"its import_runs table repeats an id",
		"it has no import_runs table",
		"its import_runs table is incomplete",
		"its import_runs table has an id too large to follow",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{historyRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_names_the_findings_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{"its findings table repeats an id", "its findings table is incomplete"}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{findingsRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_warns_of_a_findings_fault_alone_without_the_import_history_line(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{findingsRestartLine("its findings table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_prints_the_history_line_then_the_findings_line_when_both_tables_are_faulty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"}
	findings := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: history, FindingsFault: findings}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
	}, outcome.Warnings())
}

func Test_sync_and_import_warns_of_a_findings_fault_alone_when_the_store_is_flagged_unreadable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, FindingsFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{findingsRestartLine("its findings table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_names_the_rates_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{
		"its fx_rates table repeats a date",
		"its fx_rates table is incomplete",
		"its fx_rates table holds an impossible rate",
		"its fx_rates table names an unknown series",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, RatesFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{ratesRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_prints_the_history_findings_and_rates_lines_in_that_order_when_all_three_tables_are_faulty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"}
	findings := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"}
	rates := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: history, FindingsFault: findings, RatesFault: rates}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
		ratesRestartLine("its fx_rates table repeats a date"),
	}, outcome.Warnings())
}

func Test_sync_and_import_warns_of_a_rates_fault_alone_when_the_store_is_flagged_unreadable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its fx_rates table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, RatesFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{ratesRestartLine("its fx_rates table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_prints_the_combined_line_alone_when_the_unreadable_store_also_carries_a_rates_fault(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath}
	rates := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, HistoryFault: history, RatesFault: rates}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{combinedCarryLine("the file is not a DuckDB database")}, outcome.Warnings())
}

func Test_outcome_adds_no_carry_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: "quarry.duckdb"}
	unbuilt := snapshot.Outcome{Store: &store.Result{HistoryFault: fault, FindingsFault: fault, RatesFault: fault, StoreUnreadable: true}}

	assert.Empty(t, unbuilt.Warnings())
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
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: fault, StoreUnreadable: true}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], combinedCarryLine("the file is not a DuckDB database")}, outcome.Warnings())
}

func Test_import_from_puts_the_findings_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table repeats an id"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], findingsRestartLine("its findings table repeats an id")}, outcome.Warnings())
}

func Test_import_from_puts_the_rates_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its fx_rates table names an unknown series"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, RatesFault: fault}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], ratesRestartLine("its fx_rates table names an unknown series")}, outcome.Warnings())
}
