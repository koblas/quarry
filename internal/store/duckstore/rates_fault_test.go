package duckstore_test

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	reasonRatesRepeatDate = "its fx_rates table repeats a date"
	reasonRatesImpossible = "its fx_rates table holds an impossible rate"
	reasonRatesUnknown    = "its fx_rates table names an unknown series"

	// looseRatesTable is fx_rates without the primary key or CHECK a v5 store gives it, so a corrupt row can be stored.
	looseRatesTable = `DROP TABLE fx_rates CASCADE; CREATE TABLE fx_rates (date DATE, usd_cad DECIMAL(18, 6), series VARCHAR);`

	// looseRunsTable is import_runs without its primary key, so a repeated id can be stored.
	looseRunsTable = `CREATE TABLE import_runs_loose AS FROM import_runs; DROP TABLE import_runs; ALTER TABLE import_runs_loose RENAME TO import_runs;`
)

// storeWithRatesRows is a built store whose fx_rates table is looseRatesTable holding inserts, one SQL VALUES tuple per row.
func storeWithRatesRows(t *testing.T, opts []duckstore.Option, inserts ...string) *duckstore.Store {
	t.Helper()
	st := newBuiltStore(t, opts...)
	var ddl strings.Builder
	ddl.WriteString(looseRatesTable)
	for _, row := range inserts {
		ddl.WriteString(" INSERT INTO fx_rates VALUES " + row + ";")
	}
	execOnStore(t, st, ddl.String())
	return st
}

// ratesReason is the phrase a sync would print for replaced's rates fault.
func ratesReason(t *testing.T, st *duckstore.Store, replaced store.Replaced) string {
	t.Helper()
	require.NotNil(t, replaced.RatesFault)
	return replaced.RatesFault.UnreadableReason(st.Path())
}

func Test_replace_names_a_repeated_rate_date_as_the_rates_fault(t *testing.T) {
	t.Parallel()
	st := storeWithRatesRows(t, nil, "('2026-03-13', 1.25, 'IEXE0101')", "('2026-03-13', 1.26, 'FXUSDCAD')")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonRatesRepeatDate, ratesReason(t, st, replaced))
}

func Test_replace_names_an_impossible_rate_as_the_rates_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  string
	}{
		{name: "a zero rate", row: "('2026-03-13', 0, 'FXUSDCAD')"},
		{name: "a negative rate", row: "('2026-03-13', -1.25, 'FXUSDCAD')"},
		{name: "a rate one millionth past the column maximum", row: "('2026-03-13', 10000.000000, 'FXUSDCAD')"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := storeWithRatesRows(t, nil, c.row)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonRatesImpossible, ratesReason(t, st, replaced))
		})
	}
}

func Test_replace_carries_a_rate_of_one_millionth_and_one_at_the_column_maximum(t *testing.T) {
	t.Parallel()
	st := storeWithRatesRows(t, nil, "('2026-03-13', 0.000001, 'FXUSDCAD')", "('2026-03-14', 9999.999999, 'IEXE0101')")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.RatesFault)
	assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-13 0.000001 FXUSDCAD, 2026-03-14 9999.999999 IEXE0101")
}

func Test_replace_names_an_unknown_series_as_the_rates_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  string
	}{
		{name: "a name the publisher never used", row: "('2026-03-13', 1.25, 'ECB')"},
		{name: "a known name in the wrong case", row: "('2026-03-13', 1.25, 'fxusdcad')"},
		{name: "an empty name", row: "('2026-03-13', 1.25, '')"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := storeWithRatesRows(t, nil, c.row)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonRatesUnknown, ratesReason(t, st, replaced))
		})
	}
}

func Test_replace_names_an_fx_rates_table_it_cannot_read_a_cell_of_as_incomplete(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
	}{
		{name: "a NULL date", ddl: looseRatesTable + " INSERT INTO fx_rates VALUES (NULL, 1.25, 'FXUSDCAD');"},
		{name: "a NULL rate", ddl: looseRatesTable + " INSERT INTO fx_rates VALUES ('2026-03-13', NULL, 'FXUSDCAD');"},
		{name: "a NULL series", ddl: looseRatesTable + " INSERT INTO fx_rates VALUES ('2026-03-13', 1.25, NULL);"},
		{name: "no usd_cad column", ddl: "DROP TABLE fx_rates CASCADE; CREATE TABLE fx_rates (date DATE, series VARCHAR);"},
		{name: "no series column", ddl: "DROP TABLE fx_rates CASCADE; CREATE TABLE fx_rates (date DATE, usd_cad DECIMAL(10, 6));"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t)
			execOnStore(t, st, c.ddl)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonRatesIncomplete, ratesReason(t, st, replaced))
		})
	}
}

func Test_replace_carries_the_history_and_findings_when_the_rates_cannot_be_read(t *testing.T) {
	t.Parallel()
	st := storeWithRatesRows(t, nil, "('2026-03-13', 1.25, 'IEXE0101')", "('2026-03-13', 1.26, 'FXUSDCAD')")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Nil(t, replaced.FindingsFault)
	assert.True(t, replaced.FindingsCarried)
	assert.Equal(t, []int64{1, 2}, importRunIDs(t, st))
}

func Test_replace_carries_none_of_the_rates_and_asks_for_them_all_when_the_carried_rates_cannot_be_read(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(17, 1_320_000, currentSeries)}, Added: 1}}
	st := storeWithRatesRows(t, []duckstore.Option{duckstore.WithRates(src)}, "('2026-03-13', 1.25, 'IEXE0101')", "('2026-03-13', 1.26, 'FXUSDCAD')")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-17 1.320000 FXUSDCAD")
	require.Len(t, src.requests, 1)
	assert.Equal(t, store.DateSpan{}, src.requests[0].Have)
}

func Test_replace_carries_the_rates_and_their_span_when_the_import_runs_cannot_be_read(t *testing.T) {
	t.Parallel()
	const checkedFrom = "UPDATE import_runs SET rates_checked_from = DATE '2026-03-01'; "
	cases := []struct {
		name       string
		ddl        string
		wantReason string
	}{
		{name: "an id twice", ddl: looseRunsTable + " INSERT INTO import_runs FROM import_runs;", wantReason: reasonRepeatedID},
		{name: "an id twice beside a floor", ddl: checkedFrom + looseRunsTable + " INSERT INTO import_runs FROM import_runs;", wantReason: reasonRepeatedID},
		{name: "an id too large to follow beside a floor", ddl: checkedFrom + looseRunsTable + " UPDATE import_runs SET id = 9223372036854775807;", wantReason: reasonIDTooLarge},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeRates{}
			st := newBuiltStore(t, duckstore.WithRates(src))
			execOnStore(t, st, c.ddl)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, c.wantReason, historyReason(t, st, replaced))
			assert.Nil(t, replaced.RatesFault)
			assertScalar(t, openReadOnly(t, replaced.Path), fxRatesText, "2026-03-13 1.250000 IEXE0101")
			require.Len(t, src.requests, 1)
			assert.Equal(t, store.DateSpan{First: day(2026, 3, 13), Last: day(2026, 3, 13)}, src.requests[0].Have)
		})
	}
}
