package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeRecurring(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
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

func monthlyCharges(payee string, cents ...int64) store.Charges {
	rows := make([]store.Charge, len(cents))
	for i, c := range cents {
		rows[i] = store.Charge{
			TransactionID: "txn", SourceID: int64(i + 1),
			Date:    time.Date(2026, time.Month(2+i), 12, 0, 0, 0, 0, time.UTC),
			Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD"},
			PayeeID: new("payee-1"), Payee: &payee, Currency: "CAD", Amount: c, ExpenseSplits: 1,
		}
	}
	return store.Charges{Rows: rows}
}

func Test_recurring_json_prints_the_document_and_no_table(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: monthlyCharges("Netflix.com", 999, 999, 999, 999)}

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--json")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	var doc struct {
		Since  string `json:"since"`
		Series []struct {
			Payee  string `json:"payee"`
			Amount string `json:"amount"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, "2000-01-01", doc.Since)
	require.Len(t, doc.Series, 1)
	assert.Equal(t, "Netflix.com", doc.Series[0].Payee)
	assert.Equal(t, "9.99", doc.Series[0].Amount)
	assert.NotContains(t, stdout.String(), "Recurring charges")
}

func Test_recurring_without_json_prints_the_table(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: monthlyCharges("Netflix.com", 999, 999, 999, 999)}

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in all accounts")
	assert.NotContains(t, stdout.String(), `"series"`)
}

func Test_recurring_json_returns_the_report_fault_with_nothing_on_stdout(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--json")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_recurring_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     &stdout, Stderr: &stderr,
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
		LoadConfig: cadConfig,
		Stdout:     &stdout, Stderr: &stderr,
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
reference numbers count as one payee. Charges dated after today are left
out, even with a later --until.

Series are found in each account's own currency, so a change in the
exchange rate is never a price change, and a payee that charges in both
CAD and USD has two series. Amount and Per year are converted to the
reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the rate on the latest charge's date; price changes
stay in the series' own currency. With --currency native nothing is
converted.

A charge that comes off schedule starts the series again. A series has
ended when no charge has come for 14 days (weekly), 45 days (monthly), 120
days (quarterly) or 400 days (yearly). Bills whose amount changes most
times, such as hydro, are not listed; see quarry spend --by payee.

--since and --until choose which series to list: those running at any
time in the period. A series whose first charge falls in the period is
marked new. A price change is a step of more than 5% from one charge to
the next, in the series' own currency. Per year is the latest amount
times the charges in a year, for active series only.
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

func Test_recurring_reports_a_failed_stdout_write(t *testing.T) {
	err := executeRecurring(t, fakeReportStore{}, failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}
