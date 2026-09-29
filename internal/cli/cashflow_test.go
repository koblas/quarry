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

const (
	cashFlowEmpty = "no income or spending from 2026-01-01 to 2026-09-29"

	cashFlowCaption = "Cash flow 2026-01-01 to 2026-09-29 in all accounts\n\n"
)

func leftOutCashFlowWarning(name string) string {
	return "account \"" + name + "\" is not used in reports in Quicken, so cashflow leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
}

func executeCashFlow(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now: func() time.Time { return spendTagNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"cashflow"}, args...), env)
}

// withCashFlow is fake answering every cash-flow read with one CAD total, so the window is not empty.
func withCashFlow(fake fakeReportStore) fakeReportStore {
	fake.cashFlow = store.CashFlow{Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 100}}}
	return fake
}

func Test_cashflow_reads_the_months_of_the_default_window_and_heads_the_first_column_Month(t *testing.T) {
	var got store.CashFlowParams
	var stdout, stderr bytes.Buffer
	rate := 31.9
	fake := fakeReportStore{gotCashFlow: &got, cashFlow: store.CashFlow{
		Rows:   []store.CashFlowRow{{Period: "2026-09", Currency: "CAD", Income: 910000, Spent: 620000, Net: 290000, SavingsRatePct: &rate}},
		Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 910000, Spent: 620000, Net: 290000, SavingsRatePct: &rate}},
	}}

	err := executeCashFlow(t, fake, &stdout, &stderr, "--since", "2026-09")

	require.NoError(t, err)
	assert.Equal(t, store.CashFlowByMonth, got.By)
	assert.Equal(t, store.Window{
		Since: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
	}, got.Window)
	assert.Equal(t, "Cash flow 2026-09-01 to 2026-09-29 in all accounts\n\n"+
		"Month    Currency    Income     Spent       Net  Savings rate  Status\n"+
		"2026-09  CAD       9,100.00  6,200.00  2,900.00         31.9%  partial\n"+
		"Total    CAD       9,100.00  6,200.00  2,900.00         31.9%\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_cashflow_by_year_reads_the_year_period_and_heads_the_first_column_Year(t *testing.T) {
	var got store.CashFlowParams
	var stdout, stderr bytes.Buffer
	fake := withCashFlow(fakeReportStore{gotCashFlow: &got})

	err := executeCashFlow(t, fake, &stdout, &stderr, "--by", "year", "--since", "2026")

	require.NoError(t, err)
	assert.Equal(t, store.CashFlowByYear, got.By)
	assert.Equal(t, "Cash flow 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		"Year   Currency  Income  Spent   Net  Savings rate  Status\n"+
		"2026   CAD         0.00   0.00  0.00           n/a  partial\n"+
		"Total  CAD         1.00   0.00  0.00           n/a\n", stdout.String())
}

func Test_cashflow_refuses_a_by_that_names_no_period_before_reading_the_store(t *testing.T) {
	for _, by := range []string{"week", ""} {
		t.Run("--by "+by, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeCashFlow(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--by", by)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, "--by must be month or year")
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_cashflow_refuses_a_by_before_it_looks_at_the_window_or_opens_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"cashflow", "--by", "week", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, "--by must be month or year")
}

func Test_cashflow_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"cashflow", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
}

func Test_cashflow_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_cashflow_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Now:       time.Now,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"cashflow"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_cashflow_returns_a_failed_stdout_write(t *testing.T) {
	var stderr bytes.Buffer

	err := executeCashFlow(t, withCashFlow(fakeReportStore{}), failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.Empty(t, stderr.String())
}

func Test_cashflow_json_prints_the_text_table_until_the_document_exists(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, fakeReportStore{}, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Equal(t, cashFlowCaption+"Month  Currency  Income  Spent  Net  Savings rate  Status\n", stdout.String())
}

func Test_cashflow_captions_the_named_accounts_and_passes_their_ids_to_the_report(t *testing.T) {
	var got store.CashFlowParams
	var stdout, stderr bytes.Buffer
	fake := withCashFlow(namedAccounts())
	fake.gotCashFlow = &got

	err := executeCashFlow(t, fake, &stdout, &stderr, "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, []string{visaID, chequingID}, got.AccountIDs)
	assert.Contains(t, stdout.String(), "Cash flow 2026-01-01 to 2026-09-29 in Visa Infinite, Chequing\n\n")
}

func Test_cashflow_warns_once_per_named_account_left_out_of_reports_saying_cashflow_leaves_it_out(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, withCashFlow(namedAccounts()), &stdout, &stderr,
		"--account", "Old Card", "--account", chequingID, "--account", oldBankID, "--account", "old card")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutCashFlowWarning("Old Card")+"\n"+
		"quarry: warning: "+leftOutCashFlowWarning("Old Bank")+"\n", stderr.String())
}

func Test_cashflow_puts_the_empty_window_note_after_the_left_out_of_reports_warnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.cashFlow = store.CashFlow{Transactions: namedSpan}

	err := executeCashFlow(t, fake, &stdout, &stderr, "--account", "Old Card", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutCashFlowWarning("Old Card")+"\n"+
		"quarry: warning: "+cashFlowEmpty+" in the named accounts; their transactions run 2019-03-02 to 2024-11-30\n", stderr.String())
}

func Test_cashflow_says_nothing_of_an_empty_window_when_every_named_account_is_left_out_of_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, namedAccounts(), &stdout, &stderr, "--account", "Old Card", "--account", oldBankID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutCashFlowWarning("Old Card")+"\n"+
		"quarry: warning: "+leftOutCashFlowWarning("Old Bank")+"\n", stderr.String())
}

func Test_cashflow_says_when_the_window_holds_nothing_and_the_store_has_transactions_elsewhere(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{cashFlow: store.CashFlow{Transactions: storeSpan}}

	err := executeCashFlow(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+cashFlowEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

func Test_cashflow_says_nothing_of_an_empty_window_when_a_currency_nets_to_zero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{cashFlow: store.CashFlow{Totals: []store.CashFlowTotal{{Currency: "CAD"}}, Transactions: storeSpan}}

	err := executeCashFlow(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}
