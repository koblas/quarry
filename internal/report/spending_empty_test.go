package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_spend_reports_the_transaction_range_the_store_found(t *testing.T) {
	span := store.TransactionRange{
		First: time.Date(2003, 1, 4, 0, 0, 0, 0, time.UTC),
		Last:  time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	srv := report.NewServer(report.WithStore(fakeStore{spending: store.Spending{Transactions: span}}))

	got, err := srv.Spend(t.Context(), report.SpendRequest{})

	require.NoError(t, err)
	assert.Equal(t, span, got.Transactions)
}
