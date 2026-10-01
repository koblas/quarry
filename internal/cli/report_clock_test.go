package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each read of the clock returns a day later than the one before, so a command that reads it twice
// sees two different days.
func advancingClock() func() time.Time {
	reads := 0
	return func() time.Time {
		at := spendNow.Add(time.Duration(reads) * 24 * time.Hour)
		reads++
		return at
	}
}

func executeAtAdvancingClock(t *testing.T, command string, fake fakeReportStore, stdout *bytes.Buffer) error {
	t.Helper()
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: &bytes.Buffer{},
		Now: advancingClock(),
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), []string{command}, env)
}

func Test_recurring_judges_a_series_by_the_same_day_its_window_ends_on(t *testing.T) {
	var stdout bytes.Buffer
	payee := "Netflix.com"
	rows := make([]store.Charge, 0, 3)
	for i, date := range []string{"2026-06-16", "2026-07-16", "2026-08-15"} {
		day, err := time.Parse(time.DateOnly, date)
		require.NoError(t, err)
		rows = append(rows, store.Charge{
			TransactionID: "txn", SourceID: int64(i + 1), Date: day,
			Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD"},
			PayeeID: new("payee-1"), Payee: &payee, Currency: "CAD", Amount: 999, ExpenseSplits: 1,
		})
	}

	err := executeAtAdvancingClock(t, "recurring", fakeReportStore{charges: store.Charges{Rows: rows}}, &stdout)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Recurring charges 2026-01-01 to 2026-09-29 in all accounts")
	assert.Contains(t, stdout.String(), "2026-08-15  active, new")
}

func Test_anomalies_reads_charges_through_the_same_day_its_window_ends_on(t *testing.T) {
	var stdout bytes.Buffer
	var got store.ChargeParams
	fake := fakeReportStore{gotCharges: &got}

	err := executeAtAdvancingClock(t, "anomalies", fake, &stdout)

	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), got.Through)
	assert.Contains(t, stdout.String(), "Unusually large charges 2026-01-01 to 2026-09-29 in all accounts")
}
