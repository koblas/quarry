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

func civilDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func Test_spend_reads_the_period_its_since_and_until_flags_name(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want store.Window
	}{
		{
			name: "both flags cover their whole months",
			args: []string{"--since", "2025-03", "--until", "2025-05"},
			want: store.Window{Since: civilDay(2025, time.March, 1), Until: civilDay(2025, time.May, 31)},
		},
		{
			name: "since alone runs to today",
			args: []string{"--since", "2026-03"},
			want: store.Window{Since: civilDay(2026, time.March, 1), Until: civilDay(2026, time.September, 29)},
		},
		{
			name: "until alone starts on January 1 of this year",
			args: []string{"--until", "2026-12"},
			want: store.Window{Since: civilDay(2026, time.January, 1), Until: civilDay(2026, time.December, 31)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got store.SpendingParams
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{gotSpending: &got}, spendNow, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.Window)
		})
	}
}

func Test_spend_refuses_a_period_it_cannot_use_before_reading_the_store(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			"a since that is not a date",
			[]string{"--since", "2024-13"},
			`--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			"an until that is not a date",
			[]string{"--until", "2024-13"},
			`--until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			"a since after until",
			[]string{"--since", "2025", "--until", "2024"},
			"--since 2025 is after --until 2024",
		},
		{
			"an until before the default since",
			[]string{"--until", "2024"},
			"--until 2024 is before the default --since 2026-01-01; pass --since too",
		},
		{
			"a since after today",
			[]string{"--since", "2027"},
			"--since 2027 is after today; pass --until to include future-dated transactions",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{err: errStoreRead}, spendNow, &stdout, &stderr, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, c.want)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_spend_refuses_a_bad_period_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return nil, errStoreRead
		},
	}

	err := cli.Execute(t.Context(), []string{"spend", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_refuses_an_empty_period_flag_as_a_bad_date(t *testing.T) {
	cases := []struct {
		flag string
		want string
	}{
		{"--since", `--since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"--until", `--until "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
	}

	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, c.flag, "")

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, c.want)
		})
	}
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_a_bad_period(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--by", "vendor", "--since", "2024-13")

	require.EqualError(t, err, "--by must be category, payee, tag or month")
}

func Test_spend_help_shows_the_since_and_until_flags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `--since date +count transactions dated on or after date `+
		`\(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year\)`, stdout.String())
	assert.Regexp(t, `--until date +count transactions dated on or before date `+
		`\(YYYY, YYYY-MM or YYYY-MM-DD; default today\)`, stdout.String())
}

func Test_spend_help_shows_the_account_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `--account name +count only the account with this name or id; repeat for more`, stdout.String())
}
