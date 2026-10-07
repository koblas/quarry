package duckstore_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fxRatesText = `SELECT coalesce(string_agg(CAST(date AS VARCHAR) || ' ' || CAST(usd_cad AS VARCHAR) || ' ' || series, ', ' ORDER BY date), '')
FROM fx_rates`

func Test_replace_stores_each_fetched_rate_with_its_series(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{
		ratesOn(16, 1_310_000, "FXUSDCAD"), ratesOn(13, 1_250_000, "IEXE0101"),
	}}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, fxRatesText, "2026-03-13 1.250000 IEXE0101, 2026-03-16 1.310000 FXUSDCAD")
}

func Test_replace_stores_no_rates_when_it_has_no_rates_source(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM fx_rates", "0")
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM store_info", "1")
}

func Test_replace_asks_for_rates_from_the_earliest_transaction_to_today(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	rows := minimalRows()
	later := rows.Transactions[0]
	later.ID, later.Date = "txn-2", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	earlier := rows.Transactions[0]
	earlier.ID, earlier.Date = "txn-0", time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	rows.Transactions = append(rows.Transactions, later, earlier)
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))
	dayBefore := localDate(time.Now())

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	dayAfter := localDate(time.Now())
	require.Len(t, src.requests, 1)
	got := src.requests[0]
	assert.Equal(t, time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC), got.Need.First)
	assert.Contains(t, []time.Time{dayBefore, dayAfter}, got.Need.Last)
	assert.Equal(t, store.DateSpan{}, got.Have)
}

func Test_replace_asks_for_rates_from_the_earliest_investment_transaction(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	rows := minimalRows()
	earlier := rows.InvestmentTransactions[1]
	earlier.ID, earlier.Date = "inv-0", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, earlier)
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	require.Len(t, src.requests, 1)
	assert.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), src.requests[0].Need.First)
}

func Test_replace_does_not_ask_for_rates_from_a_price_dated_before_every_transaction(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	rows := minimalRows()
	rows.Prices[0].Date = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	require.Len(t, src.requests, 1)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), src.requests[0].Need.First)
}

// Moves the process zone and the clock: no t.Parallel.
func Test_replace_asks_for_rates_up_to_the_local_date_when_it_differs_from_the_utc_date(t *testing.T) {
	saved := time.Local                            //nolint:gosmopolitan // the test swaps the process-local zone; Cleanup restores it
	time.Local = time.FixedZone("UTC-5", -5*60*60) //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = saved })       //nolint:gosmopolitan // restores the zone
	src := &fakeRates{}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))
	rows := minimalRows()
	rows.Transactions[0].Date = day(1999, 12, 1)

	synctest.Test(t, func(t *testing.T) {
		time.Sleep(2 * time.Hour) // the bubble clock starts at 2000-01-01 00:00 UTC: 02:00 UTC, 21:00 on 1999-12-31 local

		_, err := st.Replace(t.Context(), rows)

		require.NoError(t, err)
	})

	require.Len(t, src.requests, 1)
	assert.Equal(t, time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC), src.requests[0].Need.Last)
}

func Test_replace_asks_for_no_dates_when_there_are_no_transactions(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), noTransactionRows())

	require.NoError(t, err)
	require.Len(t, src.requests, 1)
	assert.Equal(t, store.RatesRequest{}, src.requests[0])
}

func Test_replace_swaps_in_the_store_when_the_rate_fetch_fails(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{
		Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, FetchError: "rates host unreachable", Partial: true,
	}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, fxRatesText, "2026-03-13 1.250000 IEXE0101")
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM store_info", "1")
}

func Test_replace_swaps_in_the_store_with_no_rates_when_the_fetch_returns_none(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{FetchError: "rates host unreachable"}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM fx_rates", "0")
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM store_info", "1")
}

func Test_replace_does_no_build_work_after_a_rate_fetch_the_context_interrupted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	src := &fakeRates{
		refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}},
		during:  func(ctx context.Context) { cancel(); <-ctx.Done() },
		err:     context.Canceled,
	}
	fault := &faultDB{}
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = newFaultStore(dir, fault, duckstore.WithRates(src)).Replace(ctx, rows)

	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, fault.appended, "transactions")
	assert.NotContains(t, fault.appended, "fx_rates")
	assert.NotContains(t, fault.appended, "store_info")
	assert.Zero(t, fault.checkpoints)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_asks_for_no_rates_when_the_build_fails(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}}}
	st := newFaultStore(t.TempDir(), &faultDB{appendFaultTable: "transactions", appendFault: &duckdbdriver.Error{
		Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: Duplicate key violates primary key constraint.",
	}}, duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorContains(t, err, "load transactions")
	assert.Empty(t, src.requests)
}

func Test_replace_keeps_the_previous_store_when_the_rates_cannot_be_written(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}}}
	st := newFaultStore(dir, &faultDB{appendFaultTable: "fx_rates", appendFault: &duckdbdriver.Error{
		Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: Duplicate key \"date: 2026-03-13\" violates primary key constraint.",
	}}, duckstore.WithRates(src))
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = st.Replace(t.Context(), rows)

	require.ErrorContains(t, err, "load fx_rates")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_keeps_the_previous_store_when_a_rate_does_not_fit_the_column(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 10_000_000_000, "IEXE0101")}}}
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)

	require.ErrorContains(t, err, "rate for 2026-03-13")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_keeps_the_previous_store_when_fetched_rates_break_a_constraint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		rates   []store.Rate
		wantErr string
	}{
		{name: "two rates on one date", rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101"), ratesOn(13, 1_260_000, "FXUSDCAD")}, wantErr: "2026-03-13"},
		{name: "a zero rate", rates: []store.Rate{ratesOn(13, 0, "IEXE0101")}, wantErr: "CHECK"},
		{name: "a negative rate", rates: []store.Rate{ratesOn(13, -1_250_000, "IEXE0101")}, wantErr: "CHECK"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			before, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			src := &fakeRates{refresh: store.RatesRefresh{Rates: c.rates}}
			rows := minimalRows()
			rows.Transactions[0].Amount = 999

			_, err = duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)

			require.ErrorContains(t, err, "load fx_rates")
			require.ErrorContains(t, err, c.wantErr)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
			after, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

// latestRunRatesText reads the rates columns of the newest import run as "<checked_from>, <last>", NULL spelled out.
const latestRunRatesText = `SELECT coalesce(CAST(rates_checked_from AS VARCHAR), 'NULL') || ', ' || coalesce(CAST(rates_last AS VARCHAR), 'NULL')
FROM import_runs ORDER BY id DESC LIMIT 1`

func Test_replace_records_the_asked_floor_and_the_last_rate_on_the_new_run(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{
		ratesOn(13, 1_250_000, "IEXE0101"), ratesOn(16, 1_310_000, "FXUSDCAD"),
	}, Added: 2}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, latestRunRatesText, "2026-03-15, 2026-03-16")
}

func Test_replace_records_the_rates_columns_by_what_the_fetch_answered(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		refresh store.RatesRefresh
		rows    store.Rows
		want    string
	}{
		{name: "an empty answer still advances the floor", refresh: store.RatesRefresh{}, rows: minimalRows(), want: "2026-03-15, NULL"},
		{
			name:    "a failed fetch leaves the floor null even with a partial result",
			refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, Added: 1, FetchError: "unreachable", Partial: true},
			rows:    minimalRows(),
			want:    "NULL, 2026-03-13",
		},
		{name: "nothing was asked when there are no transactions", refresh: store.RatesRefresh{}, rows: noTransactionRows(), want: "NULL, NULL"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := duckstore.New(t.TempDir(), duckstore.WithRates(&fakeRates{refresh: c.refresh}))

			replaced, err := st.Replace(t.Context(), c.rows)

			require.NoError(t, err)
			assertScalar(t, openReadOnly(t, replaced.Path), latestRunRatesText, c.want)
		})
	}
}

// latestFetchErrorText reads the newest import run's rates_fetch_error, NULL spelled out.
const latestFetchErrorText = `SELECT coalesce(rates_fetch_error, 'NULL') FROM import_runs ORDER BY id DESC LIMIT 1`

func Test_replace_records_the_fetch_reason_on_the_new_run(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		refresh store.RatesRefresh
		want    string
	}{
		{name: "a failed fetch records its reason", refresh: store.RatesRefresh{FetchError: "cannot reach www.bankofcanada.ca"}, want: "cannot reach www.bankofcanada.ca"},
		{name: "a fetch that answered records NULL", refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, Added: 1}, want: "NULL"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := duckstore.New(t.TempDir(), duckstore.WithRates(&fakeRates{refresh: c.refresh}))

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assertScalar(t, openReadOnly(t, replaced.Path), latestFetchErrorText, c.want)
		})
	}
}

func Test_replace_leaves_the_rates_columns_of_earlier_runs_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, Added: 1}}
	_, err := duckstore.New(dir, duckstore.WithRates(first)).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	second := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD")}, Added: 1}}

	replaced, err := duckstore.New(dir, duckstore.WithRates(second)).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), `SELECT string_agg(CAST(id AS VARCHAR) || ' ' || coalesce(CAST(rates_last AS VARCHAR), 'NULL'), ', ' ORDER BY id) FROM import_runs`,
		"1 2026-03-13, 2 2026-03-16")
}

func Test_replace_reports_the_stored_rate_span_and_what_this_fetch_added(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{
		ratesOn(16, 1_310_000, "FXUSDCAD"), ratesOn(13, 1_250_000, "IEXE0101"), ratesOn(14, 1_260_000, "IEXE0101"),
	}, Added: 3, FetchError: "unreachable"}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, store.RatesSummary{
		First: time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		Added: 3, FetchError: "unreachable",
	}, replaced.Rates)
}

func Test_replace_reports_a_partial_fetch_in_its_summary(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{
		Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, Added: 1, FetchError: "unreachable", Partial: true,
	}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.True(t, replaced.Rates.Partial)
}

func Test_replace_reports_no_rate_span_when_none_are_stored(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, store.RatesSummary{}, replaced.Rates)
}

func Test_replace_keeps_the_previous_store_when_the_run_cannot_record_its_rates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	fault := &faultDB{execFaultOn: duckstore.RecordRatesQuery, execFault: &duckdbdriver.Error{
		Type: duckdbdriver.ErrorTypeIO, Msg: `IO Error: Could not write file "quarry.duckdb.partial": No space left on device`,
	}}
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = newFaultStore(dir, fault, duckstore.WithRates(&fakeRates{})).Replace(t.Context(), rows)

	require.ErrorIs(t, err, store.ErrDiskFull)
	require.ErrorContains(t, err, "record exchange rates")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(replaced.Path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_keeps_the_previous_store_when_the_stored_rates_cannot_be_read(t *testing.T) {
	t.Parallel()
	readFault := ioFault(`query rows "SELECT"`)
	cases := []struct {
		name  string
		fault *faultDB
	}{
		{name: "the query fails", fault: &faultDB{queryFaultOn: duckstore.StoredRatesQuery, queryFault: readFault}},
		{name: "the row cannot be scanned", fault: &faultDB{queryFaultOn: duckstore.StoredRatesQuery, scanFault: readFault}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			before, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)

			_, err = newFaultStore(dir, c.fault, duckstore.WithRates(&fakeRates{})).Replace(t.Context(), minimalRows())

			require.ErrorIs(t, err, readFault)
			require.ErrorContains(t, err, "read stored exchange rates")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
			after, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func localDate(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

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

func Test_replace_keeps_the_previous_store_when_the_source_answers_a_date_already_carried(t *testing.T) {
	t.Parallel()
	dir := storeWithRates(t, ratesOn(13, 1_250_000, legacySeries))
	before, err := os.ReadFile(filepath.Join(dir, duckstore.FileName))
	require.NoError(t, err)
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_260_000, currentSeries)}, Added: 1}}
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)

	require.ErrorContains(t, err, "load fx_rates")
	require.ErrorContains(t, err, "2026-03-13")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{duckstore.FileName}, direntNames(entries))
	after, err := os.ReadFile(filepath.Join(dir, duckstore.FileName))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

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

// noFloor is the day of March that stands for a NULL rates_checked_from in a seeded run.
const noFloor = 0

// floorStoreDDL is a v5-shaped import_runs (rates_checked_from last) beside an empty fx_rates.
const floorStoreDDL = `CREATE TABLE import_runs (
	id BIGINT PRIMARY KEY, started_at TIMESTAMP NOT NULL, finished_at TIMESTAMP NOT NULL,
	snapshot_path VARCHAR NOT NULL, snapshot_sha256 VARCHAR NOT NULL, schema_fingerprint VARCHAR NOT NULL,
	accounts_rows BIGINT NOT NULL, categories_rows BIGINT NOT NULL, payees_rows BIGINT NOT NULL, tags_rows BIGINT NOT NULL,
	transactions_rows BIGINT NOT NULL, splits_rows BIGINT NOT NULL, split_tags_rows BIGINT NOT NULL, transfers_rows BIGINT NOT NULL,
	balances_checked BIGINT NOT NULL, balances_mismatched BIGINT NOT NULL, splits_mismatched BIGINT NOT NULL,
	transfers_one_sided BIGINT NOT NULL, investment_transactions_not_imported BIGINT NOT NULL, rates_checked_from DATE);
CREATE TABLE fx_rates (date DATE PRIMARY KEY, usd_cad DECIMAL(10, 6) NOT NULL, series VARCHAR NOT NULL);`

// marchDate is the SQL literal for the given day of March 2026, NULL for noFloor.
func marchDate(dayOfMonth int) string {
	if dayOfMonth == noFloor {
		return "NULL"
	}
	return fmt.Sprintf("DATE '2026-03-%02d'", dayOfMonth)
}

// newStoreWithFloors is a store file with one import run per floor, ids 1.., each carrying that day of March as its
// rates_checked_from, and one FXUSDCAD rate on each of rateDays.
func newStoreWithFloors(t *testing.T, floors, rateDays []int, opts ...duckstore.Option) *duckstore.Store {
	t.Helper()
	var ddl strings.Builder
	ddl.WriteString(floorStoreDDL)
	for i, floor := range floors {
		fmt.Fprintf(&ddl, "INSERT INTO import_runs VALUES (%d, '2026-06-01 10:00:00', '2026-06-01 10:00:02', '/snapshots/%d.sqlite', 'abc', 'sha256:fp', "+
			"7, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, %s);", i+1, i+1, marchDate(floor))
	}
	for _, rateDay := range rateDays {
		fmt.Fprintf(&ddl, "INSERT INTO fx_rates VALUES (%s, 1.25, 'FXUSDCAD');", marchDate(rateDay))
	}
	return newStoreFile(t, ddl.String(), opts...)
}

// futureRows is minimalRows with its one cash transaction dated 2099-01-01, after any date the clock can show, and no investment transactions.
func futureRows() store.Rows {
	rows := minimalRows()
	rows.Transactions[0].Date = day(2099, 1, 1)
	rows.InvestmentTransactions = nil
	return rows
}

// latestFloorText is the newest import run's rates_checked_from, NULL spelled out.
const latestFloorText = `SELECT coalesce(CAST(rates_checked_from AS VARCHAR), 'NULL') FROM import_runs ORDER BY id DESC LIMIT 1`

func Test_replace_tells_the_source_the_span_from_the_checked_floor_to_the_last_carried_rate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		floors   []int
		rateDays []int
		want     store.DateSpan
	}{
		{
			name: "a floor older than the first rate starts the span", floors: []int{1}, rateDays: []int{13, 16},
			want: store.DateSpan{First: day(2026, 3, 1), Last: day(2026, 3, 16)},
		},
		{
			name: "a floor newer than the first rate leaves the first rate as the start", floors: []int{15}, rateDays: []int{13, 16},
			want: store.DateSpan{First: day(2026, 3, 13), Last: day(2026, 3, 16)},
		},
		{
			name: "a null floor leaves the first rate as the start", floors: []int{noFloor}, rateDays: []int{13, 16},
			want: store.DateSpan{First: day(2026, 3, 13), Last: day(2026, 3, 16)},
		},
		{
			name: "a newest run with a null floor claims nothing, whatever the runs before it checked", floors: []int{1, noFloor}, rateDays: []int{13},
			want: store.DateSpan{First: day(2026, 3, 13), Last: day(2026, 3, 13)},
		},
		{
			name: "the newest run's floor wins over an older run's", floors: []int{1, 5}, rateDays: []int{13},
			want: store.DateSpan{First: day(2026, 3, 5), Last: day(2026, 3, 13)},
		},
		{
			name: "a floor with no carried rates claims nothing", floors: []int{1}, rateDays: nil,
			want: store.DateSpan{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeRates{}
			st := newStoreWithFloors(t, c.floors, c.rateDays, duckstore.WithRates(src))

			_, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			require.Len(t, src.requests, 1)
			assert.Equal(t, c.want, src.requests[0].Have)
		})
	}
}

func Test_replace_keeps_the_earliest_checked_floor_across_runs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		floors  []int
		refresh store.RatesRefresh
		rows    store.Rows
		want    string
	}{
		{name: "an older floor survives a newer ask", floors: []int{1}, refresh: store.RatesRefresh{}, rows: minimalRows(), want: "2026-03-01"},
		{name: "a newer floor drops to an older ask", floors: []int{20}, refresh: store.RatesRefresh{}, rows: minimalRows(), want: "2026-03-15"},
		{name: "a null floor takes the ask", floors: []int{noFloor}, refresh: store.RatesRefresh{}, rows: minimalRows(), want: "2026-03-15"},
		{name: "a failed fetch carries the previous floor", floors: []int{1}, refresh: store.RatesRefresh{FetchError: "unreachable"}, rows: minimalRows(), want: "2026-03-01"},
		{name: "a failed fetch with no floor ever stays null", floors: []int{noFloor}, refresh: store.RatesRefresh{FetchError: "unreachable"}, rows: minimalRows(), want: "NULL"},
		{name: "no transactions carries the previous floor", floors: []int{1}, refresh: store.RatesRefresh{}, rows: noTransactionRows(), want: "2026-03-01"},
		{name: "a failed fetch after a run with no floor stays null", floors: []int{1, noFloor}, refresh: store.RatesRefresh{FetchError: "unreachable"}, rows: minimalRows(), want: "NULL"},
		{name: "only future-dated transactions leave a null floor null", floors: []int{noFloor}, refresh: store.RatesRefresh{}, rows: futureRows(), want: "NULL"},
		{name: "only future-dated transactions carry the previous floor", floors: []int{1}, refresh: store.RatesRefresh{}, rows: futureRows(), want: "2026-03-01"},
		{name: "a failed fetch carries the newest floor, not the earliest", floors: []int{5, 20}, refresh: store.RatesRefresh{FetchError: "unreachable"}, rows: minimalRows(), want: "2026-03-20"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWithFloors(t, c.floors, nil, duckstore.WithRates(&fakeRates{refresh: c.refresh}))

			replaced, err := st.Replace(t.Context(), c.rows)

			require.NoError(t, err)
			assertScalar(t, openReadOnly(t, replaced.Path), latestFloorText, c.want)
		})
	}
}

func Test_replace_keeps_the_checked_floor_on_a_partial_fetch(t *testing.T) {
	t.Parallel()
	partial := store.RatesRefresh{Rates: []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD")}, Added: 1, FetchError: "unreachable", Partial: true}
	st := newStoreWithFloors(t, []int{20}, []int{13}, duckstore.WithRates(&fakeRates{refresh: partial}))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), latestFloorText, "2026-03-20")
}

func Test_replace_without_a_rates_source_carries_the_checked_floor(t *testing.T) {
	t.Parallel()
	st := newStoreWithFloors(t, []int{1}, []int{13})

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, replaced.Path), latestFloorText, "2026-03-01")
}

func Test_replace_forgets_the_checked_floor_when_the_carried_rates_cannot_be_read(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	spy := &spyReadDB{passQueries: 3, queryFault: ioFault(`query rows "SELECT"`)}
	st := newStoreWithFloors(t, []int{1}, []int{13}, spyOpener(spy), duckstore.WithRates(src))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	require.NotNil(t, replaced.RatesFault)
	require.Len(t, src.requests, 1)
	assert.Equal(t, store.DateSpan{}, src.requests[0].Have)
	assertScalar(t, openReadOnly(t, replaced.Path), latestFloorText, "2026-03-15")
}

func Test_replace_asks_for_nothing_when_every_transaction_is_dated_after_today(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	st := newStoreWithFloors(t, []int{noFloor}, nil, duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), futureRows())

	require.NoError(t, err)
	require.Len(t, src.requests, 1)
	assert.Equal(t, store.DateSpan{}, src.requests[0].Need)
}

// haveOnTheNextSync replaces st's store once, then builds again from the result and returns the Have that second build gave its source.
func haveOnTheNextSync(t *testing.T, st *duckstore.Store) store.DateSpan {
	t.Helper()
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	next := &fakeRates{}

	_, err = duckstore.New(filepath.Dir(st.Path()), duckstore.WithRates(next)).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	require.Len(t, next.requests, 1)
	return next.requests[0].Have
}

func Test_replace_does_not_bring_back_a_floor_forgotten_after_a_rates_fault_on_the_next_sync(t *testing.T) {
	t.Parallel()
	partialTail := store.RatesRefresh{Rates: []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD")}, Added: 1, FetchError: "unreachable", Partial: true}
	spy := &spyReadDB{passQueries: 3, queryFault: ioFault(`query rows "SELECT"`)}
	st := newStoreWithFloors(t, []int{1}, []int{13}, spyOpener(spy), duckstore.WithRates(&fakeRates{refresh: partialTail}))

	have := haveOnTheNextSync(t, st)

	assert.Equal(t, store.DateSpan{First: day(2026, 3, 16), Last: day(2026, 3, 16)}, have)
}

func Test_replace_does_not_bring_back_a_floor_forgotten_with_an_absent_fx_rates_table_on_the_next_sync(t *testing.T) {
	t.Parallel()
	partialTail := store.RatesRefresh{Rates: []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD")}, Added: 1, FetchError: "unreachable", Partial: true}
	st := newStoreWithFloors(t, []int{1}, nil, duckstore.WithRates(&fakeRates{refresh: partialTail}))
	execOnStore(t, st, "DROP TABLE fx_rates CASCADE")

	have := haveOnTheNextSync(t, st)

	assert.Equal(t, store.DateSpan{First: day(2026, 3, 16), Last: day(2026, 3, 16)}, have)
}

func Test_replace_keeps_the_floor_of_an_empty_fx_rates_table_on_the_next_sync(t *testing.T) {
	t.Parallel()
	partialTail := store.RatesRefresh{Rates: []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD")}, Added: 1, FetchError: "unreachable", Partial: true}
	st := newStoreWithFloors(t, []int{1}, nil, duckstore.WithRates(&fakeRates{refresh: partialTail}))

	have := haveOnTheNextSync(t, st)

	assert.Equal(t, store.DateSpan{First: day(2026, 3, 1), Last: day(2026, 3, 16)}, have)
}

// dailySource answers FXUSDCAD with one observation for every day of every span it is asked.
type dailySource struct{}

func (dailySource) Observations(_ context.Context, series string, sp store.DateSpan) ([]fx.Observation, error) {
	var out []fx.Observation
	for date := sp.First; series == store.SeriesCurrent && !date.After(sp.Last); date = date.AddDate(0, 0, 1) {
		out = append(out, fx.Observation{Date: date, Rate: 1_300_000})
	}
	return out, nil
}

// syncOn is one sync: its earliest transaction and the clock, both in days after 2000-01-01, the bubble's start.
type syncOn struct{ transaction, clock int }

const fxRatesSpanText = `SELECT CAST(min(date) AS VARCHAR) || ' ' || CAST(max(date) AS VARCHAR) || ' ' || CAST(count(*) AS VARCHAR) FROM fx_rates`

// Moves the process zone and the clock: no t.Parallel. The zone is pinned so each bubble's clock reads as its UTC date.
func Test_a_second_sync_leaves_the_stored_rates_one_unbroken_interval(t *testing.T) {
	saved := time.Local                      //nolint:gosmopolitan // the test pins the process-local zone; Cleanup restores it
	time.Local = time.UTC                    //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = saved }) //nolint:gosmopolitan // restores the zone
	cases := []struct {
		name          string
		first, second syncOn
	}{
		{name: "the second sync reaches back before the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 0, clock: 20}},
		{name: "the second sync reaches forward past the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 10, clock: 25}},
		{name: "the second sync reaches both ways", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 0, clock: 25}},
		{name: "the second sync needs only days already stored", first: syncOn{transaction: 0, clock: 20}, second: syncOn{transaction: 5, clock: 12}},
		{name: "the second sync's earliest transaction is after the stored rates", first: syncOn{transaction: 0, clock: 5}, second: syncOn{transaction: 10, clock: 20}},
		{name: "the clock moves back so the second sync needs only days before the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: -12, clock: 0}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := duckstore.New(t.TempDir(), duckstore.WithRates(fx.NewServer(fx.WithSource(dailySource{}))))
			sync := func(s syncOn) store.Replaced {
				var replaced store.Replaced
				synctest.Test(t, func(t *testing.T) {
					time.Sleep(time.Duration(s.clock) * 24 * time.Hour)
					rows := minimalRows()
					rows.Transactions[0].Date = day(2000, 1, 1).AddDate(0, 0, s.transaction)

					var err error
					replaced, err = st.Replace(t.Context(), rows)

					require.NoError(t, err)
				})
				return replaced
			}
			sync(c.first)

			replaced := sync(c.second)

			first, last := min(c.first.transaction, c.second.transaction), max(c.first.clock, c.second.clock)
			wantFirst, wantLast := day(2000, 1, 1).AddDate(0, 0, first), day(2000, 1, 1).AddDate(0, 0, last)
			assertScalar(t, openReadOnly(t, replaced.Path), fxRatesSpanText,
				fmt.Sprintf("%s %s %d", wantFirst.Format(time.DateOnly), wantLast.Format(time.DateOnly), last-first+1))
		})
	}
}
