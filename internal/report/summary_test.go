package report_test

import (
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryNow is the clock the summary tests read: 2026-10-06 noon UTC, after September 2026 ended.
var summaryNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func Test_summary_holds_the_months_anomalies_new_recurring_and_net_worth_change(t *testing.T) {
	hardware := []chargeOpt{paidTo("payee-hw", "Hardware")}
	charges := slices.Concat(
		earlierCharges(t, 3, 6000, hardware...),
		[]store.Charge{chargeOn(t, 0, "2026-09-14", append(hardware, ofAmount(20000))...)},
		monthlyEndingOn(t, "2026-09-03", 3, paidTo("payee-tv", "Streaming"), ofAmount(2259)),
	)
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	august, september := day(2026, time.August, 31), day(2026, time.September, 30)
	chequing := func(date time.Time, cents int64) store.NetWorthRow {
		row := typedRow("chequing", "CAD", cents, big.NewInt(cents))
		row.Date = date
		return row
	}
	srv := report.NewServer(report.WithStore(fakeStore{
		charges:  store.Charges{Rows: charges},
		netWorth: store.NetWorth{Rows: []store.NetWorthRow{chequing(august, 100000), chequing(september, 150000)}},
	}))
	month, err := report.ParseMonth(new("2026-09"), summaryNow)
	require.NoError(t, err)

	got, err := srv.Summary(t.Context(), report.SummaryRequest{Month: month, Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, []int64{20000}, amountsOf(got.Anomalies.Listed))
	assert.Equal(t, []string{"Streaming"}, payeesOf(got.Recurring))
	require.Len(t, got.NetWorth.Dates, 2)
	assert.Equal(t, []time.Time{august, september}, []time.Time{got.NetWorth.Dates[0].Date, got.NetWorth.Dates[1].Date})
	require.NotNil(t, got.Change)
	assert.Equal(t, []report.NetWorthChangeType{{Type: "chequing", Currency: "CAD", Value: big.NewInt(50000)}}, got.Change.Types)
	assert.Equal(t, []report.NetWorthChangeTotal{{Currency: "CAD", Value: big.NewInt(50000)}}, got.Change.Totals)
}
