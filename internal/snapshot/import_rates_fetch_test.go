package snapshot_test

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fetchReason = "www.bankofcanada.ca answered 503 Service Unavailable"

var (
	firstRate = time.Date(2017, time.January, 3, 0, 0, 0, 0, time.UTC)
	lastRate  = time.Date(2017, time.January, 4, 0, 0, 0, 0, time.UTC)
)

func nothingStoredLine(reason string) string {
	return "could not fetch exchange rates from the Bank of Canada: " + reason +
		"; the store has no rates, so reports list amounts in each account's own currency; run quarry sync again to retry"
}

func nothingNewLine(reason string) string {
	return "could not fetch exchange rates from the Bank of Canada: " + reason +
		"; the store has rates from 2017-01-03 to 2017-01-04, and later dates convert at the 2017-01-04 rate; run quarry sync again to retry"
}

func partialFetchLine(reason string) string {
	return "could not fetch every exchange rate from the Bank of Canada: " + reason +
		"; the store has rates from 2017-01-03 to 2017-01-04; run quarry sync again to fetch the rest"
}

func fetchFailed() store.RatesSummary {
	return store.RatesSummary{FetchError: fetchReason}
}

func Test_sync_and_import_names_the_failed_rate_fetch_in_a_warning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		rates store.RatesSummary
		want  string
	}{
		{"nothing stored", fetchFailed(), nothingStoredLine(fetchReason)},
		{
			"nothing new, rates stored",
			store.RatesSummary{First: firstRate, Last: lastRate, FetchError: fetchReason},
			nothingNewLine(fetchReason),
		},
		{
			"some rates fetched, the rest not",
			store.RatesSummary{First: firstRate, Last: lastRate, Added: 2, FetchError: fetchReason, Partial: true},
			partialFetchLine(fetchReason),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: c.rates}})

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			assert.Equal(t, []string{c.want}, outcome.Warnings())
			assert.Equal(t, []string{c.want}, outcome.WarningsAbsolute())
		})
	}
}

func Test_sync_and_import_names_the_fetch_reason_it_was_given(t *testing.T) {
	t.Parallel()
	rates := store.RatesSummary{FetchError: "cannot reach www.bankofcanada.ca"}
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: rates}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []string{nothingStoredLine("cannot reach www.bankofcanada.ca")}, outcome.Warnings())
}

func Test_sync_and_import_adds_no_fetch_warning_when_the_rate_fetch_succeeded(t *testing.T) {
	t.Parallel()
	rates := store.RatesSummary{First: firstRate, Last: lastRate, Added: 2}
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: rates}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_outcome_adds_no_fetch_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Rates: fetchFailed()}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_outcome_lists_the_fetch_warning_before_the_prune_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, Rates: fetchFailed()}},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{nothingStoredLine(fetchReason), deleteFailureLine(ids[0], "permission denied")}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_the_fetch_warning_after_the_rates_carry_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:      true,
		RatesFault: &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"},
		Rates:      fetchFailed(),
	}
	srv := newImportServer(t, home, &fakeImporter{result: result})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{ratesRestartLine("its fx_rates table repeats a date"), nothingStoredLine(fetchReason)}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_the_fetch_warning_after_the_combined_carry_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:           true,
		StoreUnreadable: true,
		HistoryFault:    &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath},
		Rates:           fetchFailed(),
	}
	srv := newImportServer(t, home, &fakeImporter{result: result})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{combinedCarryLine("the file is not a DuckDB database"), nothingStoredLine(fetchReason)}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_every_warning_in_the_ruled_order(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:         true,
		HistoryFault:  &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"},
		FindingsFault: &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"},
		RatesFault:    &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"},
		Rates:         store.RatesSummary{First: firstRate, Last: lastRate, Added: 2, FetchError: fetchReason, Partial: true},
	}
	srv := newImportServer(t, home, &fakeImporter{result: result},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.ExtraSchemaBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	require.Len(t, outcome.Manifest.Warnings, 1)
	want := []string{
		outcome.Manifest.Warnings[0],
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
		ratesRestartLine("its fx_rates table repeats a date"),
		partialFetchLine(fetchReason),
		deleteFailureLine(ids[0], "permission denied"),
	}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}
