package duckstore_test

import (
	"context"
	"os"
	"testing"
	"testing/synctest"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fxRatesText = `SELECT coalesce(string_agg(CAST(date AS VARCHAR) || ' ' || CAST(usd_cad AS VARCHAR) || ' ' || series, ', ' ORDER BY date), '')
FROM fx_rates`

// fakeRates is a RatesSource that records each request and plays back one refresh.
type fakeRates struct {
	requests []store.RatesRequest
	refresh  store.RatesRefresh
	err      error
	during   func(ctx context.Context) // runs inside Refresh, before it returns
}

func (f *fakeRates) Refresh(ctx context.Context, req store.RatesRequest) (store.RatesRefresh, error) {
	f.requests = append(f.requests, req)
	if f.during != nil {
		f.during(ctx)
	}
	return f.refresh, f.err
}

func ratesOn(day int, usdCAD money.Rate, series string) store.Rate {
	return store.Rate{Date: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), USDCAD: usdCAD, Series: series}
}

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

// Moves the process zone and the clock: no t.Parallel.
func Test_replace_asks_for_rates_up_to_the_local_date_when_it_differs_from_the_utc_date(t *testing.T) {
	saved := time.Local                            //nolint:gosmopolitan // the test swaps the process-local zone; Cleanup restores it
	time.Local = time.FixedZone("UTC-5", -5*60*60) //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = saved })       //nolint:gosmopolitan // restores the zone
	src := &fakeRates{}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	synctest.Test(t, func(t *testing.T) {
		time.Sleep(2 * time.Hour) // the bubble clock starts at 2000-01-01 00:00 UTC: 02:00 UTC, 21:00 on 1999-12-31 local

		_, err := st.Replace(t.Context(), minimalRows())

		require.NoError(t, err)
	})

	require.Len(t, src.requests, 1)
	assert.Equal(t, time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC), src.requests[0].Need.Last)
}

func Test_replace_asks_for_no_dates_when_there_are_no_transactions(t *testing.T) {
	t.Parallel()
	src := &fakeRates{}
	rows := minimalRows()
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil, nil
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))

	_, err := st.Replace(t.Context(), rows)

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
		noTxns  bool
		want    string
	}{
		{name: "an empty answer still advances the floor", refresh: store.RatesRefresh{}, want: "2026-03-15, NULL"},
		{
			name:    "a failed fetch leaves the floor null even with a partial result",
			refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}, Added: 1, FetchError: "unreachable", Partial: true},
			want:    "NULL, 2026-03-13",
		},
		{name: "nothing was asked when there are no transactions", refresh: store.RatesRefresh{}, noTxns: true, want: "NULL, NULL"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := minimalRows()
			if c.noTxns {
				rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil, nil
			}
			st := duckstore.New(t.TempDir(), duckstore.WithRates(&fakeRates{refresh: c.refresh}))

			replaced, err := st.Replace(t.Context(), rows)

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
