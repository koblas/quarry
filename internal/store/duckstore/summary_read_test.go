package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryDates are the days the summary tests read net worth on.
var summaryDates = []time.Time{day(2026, time.March, 14), day(2026, time.March, 15)}

// summaryRows is chargeRowsFor with the import run Status needs.
func summaryRows() store.Rows {
	rows := chargeRowsFor()
	rows.ImportRuns = minimalRows().ImportRuns
	return rows
}

func Test_summary_equals_the_separate_reads(t *testing.T) {
	t.Parallel()
	rows := summaryRows()
	addCharge(&rows, oneSplit("through", 1, 100))
	afterThrough := oneSplit("after-through", 2, 200)
	afterThrough.date = chargesThrough.AddDate(0, 0, 1)
	addCharge(&rows, afterThrough)
	st := newStoreWith(t, rows)
	wantStatus, err := st.Status(t.Context())
	require.NoError(t, err)
	wantCharges, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})
	require.NoError(t, err)
	wantNetWorth, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: summaryDates})
	require.NoError(t, err)

	got, err := st.Summary(t.Context(), store.SummaryParams{Through: chargesThrough, Dates: summaryDates})

	require.NoError(t, err)
	assert.Equal(t, store.Summary{Status: wantStatus, Charges: wantCharges, NetWorth: wantNetWorth}, got)
	assert.Equal(t, []int64{100}, amountsOf(got.Charges))
	assert.NotEmpty(t, got.NetWorth.Rows)
	assert.NotEmpty(t, got.Status.Accounts)
}

func Test_summary_does_not_fail_on_the_month_ends_around_the_first_day_of_year_one(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, summaryRows())
	dates := []time.Time{day(0, time.December, 31), day(1, time.January, 31)}

	got, err := st.Summary(t.Context(), store.SummaryParams{Through: day(1, time.January, 31), Dates: dates})

	require.NoError(t, err)
	assert.Empty(t, got.NetWorth.Rows)
	assert.Empty(t, got.Charges.Rows)
}

// summaryQueries is how many queries Summary runs after the open's format checks.
const summaryQueries = 12

func Test_summary_returns_each_querys_fault(t *testing.T) {
	t.Parallel()
	for passed := range summaryQueries {
		t.Run(fmt.Sprintf("after %d queries", passed), func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault, passQueries: passed}))

			_, err := st.Summary(t.Context(), store.SummaryParams{Through: chargesThrough, Dates: summaryDates})

			assertOtherFault(t, err, "disk read failed")
			assert.ErrorIs(t, err, fault)
		})
	}
}

func Test_summary_runs_every_read_over_one_open(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{}
	st := newBuiltStore(t, spyOpener(spy))

	_, err := st.Summary(t.Context(), store.SummaryParams{Through: chargesThrough, Dates: summaryDates})

	require.NoError(t, err)
	assert.Equal(t, summaryQueries, spy.queries)
}
