package duckstore_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// noTransactionRows is minimalRows without any cash or investment transaction.
func noTransactionRows() store.Rows {
	rows := minimalRows()
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers, rows.InvestmentTransactions = nil, nil, nil, nil, nil
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
