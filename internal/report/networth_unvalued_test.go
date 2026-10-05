package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_networth_carries_the_unvalued_holdings_of_the_days_listed_and_drops_others(t *testing.T) {
	listed := store.UnvaluedHolding{Date: netWorthDay, AccountID: "acct-1", SecurityID: "sec-1"}
	january := store.UnvaluedHolding{Date: day(2026, time.January, 31), AccountID: "acct-1", SecurityID: "sec-2"}
	unrequested := store.UnvaluedHolding{Date: day(2026, time.January, 30), AccountID: "acct-1", SecurityID: "sec-3"}
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Unvalued: []store.UnvaluedHolding{january, unrequested, listed}}}))

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay, Window: &netWorthHistory})

	require.NoError(t, err)
	assert.Equal(t, []store.UnvaluedHolding{january, listed}, result.Unvalued)
}

func Test_networth_snapshot_carries_the_unvalued_holdings_from_its_one_store_read(t *testing.T) {
	held := store.UnvaluedHolding{Date: netWorthDay, AccountID: "acct-1", SecurityID: "sec-1"}
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{netWorthReads: &reads, netWorth: store.NetWorth{Unvalued: []store.UnvaluedHolding{held}}}))

	result, err := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: netWorthDay})

	require.NoError(t, err)
	assert.Equal(t, []store.UnvaluedHolding{held}, result.Unvalued)
	assert.Equal(t, 1, reads)
}
