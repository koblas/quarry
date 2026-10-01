package duckstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	legacySeries  = "IEXE0101"
	currentSeries = "FXUSDCAD"

	reasonRatesIncomplete = "its fx_rates table is incomplete"
)

// cancelOnQuery returns a read hook that calls cancel when the nth read query starts.
func cancelOnQuery(n int, cancel context.CancelFunc) func() {
	queries := 0
	return func() {
		queries++
		if queries == n {
			cancel()
		}
	}
}

// storeWithRates builds a store in a fresh directory whose fx_rates hold rates, and returns the directory.
func storeWithRates(t *testing.T, rates ...store.Rate) string {
	t.Helper()
	dir := t.TempDir()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: rates, Added: len(rates)}}
	_, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	return dir
}

func Test_replace_carries_the_rates_of_the_previous_store(t *testing.T) {
	t.Parallel()
	dir := storeWithRates(t, ratesOn(13, 1_250_001, legacySeries), ratesOn(16, 1_310_001, currentSeries))

	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-13 1.250001 IEXE0101, 2026-03-16 1.310001 FXUSDCAD")
	assert.Nil(t, replaced.RatesFault)
}

func Test_replace_reports_the_carried_rate_span_with_nothing_added(t *testing.T) {
	t.Parallel()
	dir := storeWithRates(t, ratesOn(13, 1_250_001, legacySeries), ratesOn(16, 1_310_001, currentSeries))

	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, store.RatesSummary{First: day(2026, 3, 13), Last: day(2026, 3, 16)}, replaced.Rates)
}

func Test_replace_stores_the_fetched_rates_beside_the_carried_ones(t *testing.T) {
	t.Parallel()
	dir := storeWithRates(t, ratesOn(13, 1_250_000, legacySeries))
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(17, 1_320_000, currentSeries)}, Added: 1}}

	replaced, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-13 1.250000 IEXE0101, 2026-03-17 1.320000 FXUSDCAD")
	assert.Equal(t, store.RatesSummary{First: day(2026, 3, 13), Last: day(2026, 3, 17), Added: 1}, replaced.Rates)
}

func Test_replace_tells_the_source_what_the_carried_rates_already_cover(t *testing.T) {
	t.Parallel()
	dir := storeWithRates(t, ratesOn(16, 1_310_000, currentSeries), ratesOn(13, 1_250_000, legacySeries), ratesOn(14, 1_260_000, legacySeries))
	src := &fakeRates{}

	_, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	require.Len(t, src.requests, 1)
	assert.Equal(t, store.DateSpan{First: day(2026, 3, 13), Last: day(2026, 3, 16)}, src.requests[0].Have)
}

func Test_replace_carries_no_rates_and_stays_silent_from_a_store_without_fx_rates(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, phase1ImportRunsDDL)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.RatesFault)
	assert.Equal(t, store.RatesSummary{}, replaced.Rates)
	assertScalar(t, openReadOnly(t, replaced.Path), "SELECT CAST(count(*) AS VARCHAR) FROM fx_rates", "0")
}

func Test_replace_names_a_failed_rates_read_as_the_rates_fault_alone_and_closes_the_connection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spy  *spyReadDB
	}{
		{name: "the columns query", spy: &spyReadDB{passQueries: 4, queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a column name scan", spy: &spyReadDB{passQueries: 4, scanFault: ioFault(`scan column`)}},
		{name: "the rows query", spy: &spyReadDB{passQueries: 5, queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a row scan", spy: &spyReadDB{passQueries: 5, scanFault: ioFault(`scan row`)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			require.NotNil(t, replaced.RatesFault)
			assert.Equal(t, reasonRatesIncomplete, replaced.RatesFault.UnreadableReason(st.Path()))
			assert.Nil(t, replaced.HistoryFault)
			assert.Nil(t, replaced.FindingsFault)
			assert.True(t, replaced.FindingsCarried)
			assert.Equal(t, 1, c.spy.closes)
			assertScalar(t, openReadOnly(t, st.Path()), "SELECT CAST(count(*) AS VARCHAR) FROM fx_rates", "0")
		})
	}
}

func Test_replace_does_not_swap_when_the_context_ends_during_the_rates_read(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	st := newBuiltStore(t, spyOpener(&spyReadDB{onQuery: cancelOnQuery(5, cancel)}))
	before, err := os.ReadFile(st.Path())
	require.NoError(t, err)

	_, err = st.Replace(ctx, minimalRows())

	require.ErrorIs(t, err, context.Canceled)
	after, readErr := os.ReadFile(st.Path())
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	entries, err := os.ReadDir(filepath.Dir(st.Path()))
	require.NoError(t, err)
	assert.Equal(t, []string{duckstore.FileName}, direntNames(entries))
}

func Test_replace_reports_no_rates_fault_for_a_healthy_carry(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.RatesFault)
	assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-13 1.250000 IEXE0101")
}
