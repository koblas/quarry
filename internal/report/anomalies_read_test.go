package report_test

import (
	"context"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_anomalies_read_the_charges_once_through_today_whatever_the_window(t *testing.T) {
	var got store.ChargeParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got, chargesReads: &reads}))
	window := store.Window{Since: allTime.Since, Until: dateOf(t, "2030-12-31")}

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_anomalies_read_the_charges_through_the_local_date_when_the_utc_date_is_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: windowNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_anomalies_return_the_window_and_the_transaction_span_of_the_read(t *testing.T) {
	span := store.TransactionRange{First: dateOf(t, "2003-01-04"), Last: dateOf(t, "2026-09-26")}
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Transactions: span}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, thisYear, got.Window)
	assert.Equal(t, span, got.Transactions)
}

func Test_anomalies_refuse_a_store_that_cannot_be_opened(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: openErr}), report.WithHome(refusalHome))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it", refusal.Error())
}

func Test_anomalies_pass_on_a_charges_read_fault_that_is_not_a_refusal(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: errDiskRead}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	require.ErrorIs(t, err, errDiskRead)
	_, refused := errors.AsType[report.RefusalError](err)
	assert.False(t, refused)
}

func Test_anomalies_report_an_interrupt_during_the_charges_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: errDiskRead}))

	_, err := srv.Anomalies(ctx, report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	assert.EqualError(t, err, "anomalies interrupted")
}

func Test_anomalies_list_charges_dated_on_the_window_bounds_and_not_outside_them(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-09-02"), Until: dateOf(t, "2026-09-10")}
	cases := []struct {
		name        string
		date        string
		wantListed  []int64
		wantChecked int
	}{
		{name: "the day before since is history only", date: "2026-09-01", wantListed: []int64{}, wantChecked: 0},
		{name: "a charge on since is listed", date: "2026-09-02", wantListed: []int64{50000}, wantChecked: 1},
		{name: "a charge on until is listed", date: "2026-09-10", wantListed: []int64{50000}, wantChecked: 1},
		{name: "the day after until is not listed", date: "2026-09-11", wantListed: []int64{}, wantChecked: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 10000), chargeOn(t, 9, c.date, ofAmount(50000)))

			got := anomaliesIn(t, window, charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantChecked, got.Checked)
		})
	}
}

func Test_anomalies_count_a_charge_the_day_before_the_window_as_history(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-09-02"), Until: dateOf(t, "2026-09-10")}
	charges := append(earlierCharges(t, 2, 10000),
		chargeOn(t, 3, "2026-09-01", ofAmount(10000)),
		chargeOn(t, 9, "2026-09-02", ofAmount(50000)))

	got := anomaliesIn(t, window, charges)

	assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
}

func Test_anomalies_list_newest_first_then_by_descending_source_id(t *testing.T) {
	cases := []struct {
		name             string
		dateA, dateB     string
		sourceA, sourceB int64
		wantSourceIDs    []int64
	}{
		{name: "same day, lower id from the earlier group", dateA: "2026-09-01", dateB: "2026-09-01", sourceA: 20, sourceB: 30, wantSourceIDs: []int64{30, 20}},
		{name: "same day, higher id from the earlier group", dateA: "2026-09-01", dateB: "2026-09-01", sourceA: 30, sourceB: 20, wantSourceIDs: []int64{30, 20}},
		{name: "the later date wins over a higher id, later in the second group", dateA: "2026-09-01", dateB: "2026-09-05", sourceA: 30, sourceB: 20, wantSourceIDs: []int64{20, 30}},
		{name: "the later date wins over a higher id, later in the first group", dateA: "2026-09-05", dateB: "2026-09-01", sourceA: 20, sourceB: 30, wantSourceIDs: []int64{20, 30}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 10000, paidTo("payee-a", "A")), earlierCharges(t, 3, 10000, paidTo("payee-b", "B"))...)
			charges = append(charges,
				chargeOn(t, c.sourceA, c.dateA, paidTo("payee-a", "A"), ofAmount(50000)),
				chargeOn(t, c.sourceB, c.dateB, paidTo("payee-b", "B"), ofAmount(50000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			ids := []int64{}
			for _, a := range got.Listed {
				ids = append(ids, a.SourceID)
			}
			assert.Equal(t, c.wantSourceIDs, ids)
		})
	}
}
