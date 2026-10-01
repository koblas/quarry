package cli_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeRecurring(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"recurring"}, args...), env)
}

func Test_recurring_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_recurring_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"recurring", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
}

func Test_recurring_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"recurring"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_recurring_help_says_what_recurring_lists(t *testing.T) {
	const long = `List charges that repeat on a schedule: the same payee and currency every
week, month, quarter or year, at a steady amount. quarry finds them in all
your history, with the rules of quarry spend: expense splits only, without
transfers, refunds or accounts left out of reports. A transaction counts
once, with all its splits. Payees whose names differ only in store or
reference numbers count as one payee.

A charge that comes off schedule starts the series again. A series has
ended when no charge has come for 14 days (weekly), 45 days (monthly), 120
days (quarterly) or 400 days (yearly). Bills whose amount changes most
times, such as hydro, are not listed; see quarry spend --by payee.

--since and --until choose which series to list: those running at any
time in the period. A series whose first charge falls in the period is
marked new. A price change is a step of more than 5% from one charge to
the next. Per year is the latest amount times the charges in a year, for
active series only.
`
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_recurring_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry recurring
  quarry recurring --since 2026-09 --until 2026-09 --json
  quarry recurring --since 2000
`
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_recurring_help_shows_each_flag(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{
			name: "--since",
			want: `--since date +list series running on or after date \(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year\)`,
		},
		{
			name: "--until",
			want: `--until date +list series that started on or before date \(YYYY, YYYY-MM or YYYY-MM-DD; default today\)`,
		},
		{
			name: "--account",
			want: `--account name +list only series with a charge in the account with this name or id; repeat for more`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr, "--help")

			require.NoError(t, err)
			assert.Regexp(t, c.want, stdout.String())
		})
	}
}
