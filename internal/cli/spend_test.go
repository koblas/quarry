package cli_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// utcMinus5 is a zone whose evening is already the next day in UTC.
var utcMinus5 = time.FixedZone("UTC-5", -5*60*60)

func executeSpend(t *testing.T, fake fakeReportStore, now time.Time, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now: func() time.Time { return now },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"spend"}, args...), env)
}

func Test_spend_reads_the_window_from_the_env_clock(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 12, 31, 22, 0, 0, 0, utcMinus5)

	fake := withSpending(fakeReportStore{gotSpending: &got})

	err := executeSpend(t, fake, now, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
	}, got.Window)
	assert.Equal(t, store.SpendByCategory, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-12-31 in all accounts\n\nCategory  Currency  Spent\nTotal     CAD        1.00\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_by_payee_reads_the_payee_grouping_and_heads_the_first_column_Payee(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{gotSpending: &got, spending: store.Spending{
		Rows:   []store.SpendingRow{{Key: nil, Currency: "CAD", Spent: 4208}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 4208}},
	}}

	err := executeSpend(t, fake, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), &stdout, &stderr, "--by", "payee")

	require.NoError(t, err)
	assert.Equal(t, store.SpendByPayee, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		"Payee       Currency  Spent\n"+
		"(no payee)  CAD       42.08\n"+
		"Total       CAD       42.08\n", stdout.String())
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_reading_the_store(t *testing.T) {
	for _, by := range []string{"vendor", "", "Payee"} {
		t.Run(by, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{err: errStoreRead}, time.Now(), &stdout, &stderr, "--by", by)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, "--by must be category, payee, tag or month")
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now: time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) {
			return nil, errStoreRead
		},
	}

	err := cli.Execute(t.Context(), []string{"spend", "--by", "vendor"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, "--by must be category, payee, tag or month")
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_json_puts_the_report_window_and_rows_in_the_document(t *testing.T) {
	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fake := fakeReportStore{spending: store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Auto:Fuel"), Currency: "CAD", Spent: 120450}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 120450}},
	}}

	err := executeSpend(t, fake, now, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category","account_filter":[],
		"rows":[{"category":"Auto:Fuel","currency":"CAD","spent":"1204.50"}],
		"totals":[{"currency":"CAD","spent":"1204.50"}],"warnings":[]}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{err: errStoreRead}, time.Now(), &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"spend"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_spend_reports_a_failed_stdout_write(t *testing.T) {
	err := executeSpend(t, fakeReportStore{}, time.Now(), failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}
