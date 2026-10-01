package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeAnomalies(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"anomalies"}, args...), env)
}

func Test_anomalies_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_anomalies_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"anomalies"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_anomalies_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"anomalies", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
}

func Test_anomalies_takes_no_arguments(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "extra")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, "anomalies takes no arguments")
}

func Test_anomalies_help_says_what_anomalies_lists(t *testing.T) {
	const long = `List charges that are unusually large: more than 2 times the median of the
payee's earlier charges, when there are at least 3, or else more than 5
times the median of the category's earlier charges, when there are at least
10. Charges under 100.00 are never listed. Charges follow the rules of
quarry spend, and a transaction counts once, with all its splits; an
uncategorized or split charge from a payee with little history cannot be
judged. Possible duplicates are listed by quarry findings, not here. Charges
dated after today are left out, even with a later --until.

--since and --until choose which charges to list; each is compared with
every earlier charge, however old. --account lists only charges in those
accounts; the payee's charges in other accounts still count as history.
`
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_anomalies_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry anomalies
  quarry anomalies --since 2026-09 --until 2026-09
  quarry anomalies --account "Visa Infinite" --json
`
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_anomalies_without_charges_prints_the_empty_table_and_footer_and_names_the_stores_span(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, ""+
		"Unusually large charges 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		"Date  Account  Payee  Category  Amount  Usual  Times  Compared with\n\n"+
		"0 charges checked\n", stdout.String())
	assert.Equal(t, "quarry: warning: "+anomaliesEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

func Test_anomalies_charges_exist_but_none_unusual_prints_no_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Rows: ordinaryCharge()}}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n1 charge checked\n"), stdout.String())
}

func Test_anomalies_reports_a_failed_stdout_write(t *testing.T) {
	err := executeAnomalies(t, fakeReportStore{}, failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}
