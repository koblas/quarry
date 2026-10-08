package cli_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

// spendNow is the clock every report command test runs at: 2026-09-29 noon UTC.
var spendNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// fakeReportStore answers each read with a canned result or err, recording its arguments in the
// matching got* field when set; Status panics through the nil embedded interface.
type fakeReportStore struct {
	report.Store

	accounts     store.AccountList
	spending     store.Spending
	cashFlow     store.CashFlow
	charges      store.Charges
	findings     store.FindingList
	holdings     store.Holdings
	history      store.InvestmentHistory
	netWorth     store.NetWorth
	summary      store.Summary
	result       store.QueryResult
	gotQuery     *string
	gotMaxRows   *int
	gotSpending  *store.SpendingParams
	gotCashFlow  *store.CashFlowParams
	gotCharges   *store.ChargeParams
	gotHoldings  *store.HoldingsParams
	gotSummary   *store.SummaryParams
	chargesReads *int
	err          error
}

func (f fakeReportStore) Accounts(context.Context) (store.AccountList, error) {
	return f.accounts, f.err
}

func (f fakeReportStore) Spending(_ context.Context, params store.SpendingParams) (store.Spending, error) {
	if f.gotSpending != nil {
		*f.gotSpending = params
	}
	return f.spending, f.err
}

func (f fakeReportStore) CashFlow(_ context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	if f.gotCashFlow != nil {
		*f.gotCashFlow = params
	}
	return f.cashFlow, f.err
}

func (f fakeReportStore) Charges(_ context.Context, params store.ChargeParams) (store.Charges, error) {
	if f.gotCharges != nil {
		*f.gotCharges = params
	}
	if f.chargesReads != nil {
		*f.chargesReads++
	}
	return f.charges, f.err
}

func (f fakeReportStore) Holdings(_ context.Context, params store.HoldingsParams) (store.Holdings, error) {
	if f.gotHoldings != nil {
		*f.gotHoldings = params
	}
	return f.holdings, f.err
}

func (f fakeReportStore) InvestmentHistory(context.Context) (store.InvestmentHistory, error) {
	return f.history, f.err
}

func (f fakeReportStore) Findings(context.Context) (store.FindingList, error) {
	return f.findings, f.err
}

func (f fakeReportStore) Query(_ context.Context, query string, maxRows int) (store.QueryResult, error) {
	if f.gotQuery != nil {
		*f.gotQuery = query
	}
	if f.gotMaxRows != nil {
		*f.gotMaxRows = maxRows
	}
	return f.result, f.err
}

// failingWriter fails every write with err.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// span is the range of transaction dates from first to last, both YYYY-MM-DD.
func span(t *testing.T, first, last string) store.TransactionRange {
	t.Helper()
	from, err := time.Parse(time.DateOnly, first)
	require.NoError(t, err)
	to, err := time.Parse(time.DateOnly, last)
	require.NoError(t, err)
	return store.TransactionRange{First: from, Last: to}
}

func (f fakeReportStore) NetWorth(context.Context, store.NetWorthParams) (store.NetWorth, error) {
	return f.netWorth, f.err
}

func (f fakeReportStore) Summary(_ context.Context, params store.SummaryParams) (store.Summary, error) {
	if f.gotSummary != nil {
		*f.gotSummary = params
	}
	return f.summary, f.err
}

// envOption adjusts an Env built by reportEnv or failingReportEnv.
type envOption func(*cli.Env)

// atSpendNow pins the Env clock to spendNow.
func atSpendNow(env *cli.Env) { atTime(spendNow)(env) }

// atWallClock gives the Env the real clock, for runs that fail before a window is read.
func atWallClock(env *cli.Env) { env.Now = time.Now }

// reportEnv is an Env with a CAD config whose report store is fake; its clock stays unset unless opts set it.
func reportEnv(fake report.Store, stdout, stderr io.Writer, opts ...envOption) cli.Env {
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	for _, opt := range opts {
		opt(&env)
	}
	return env
}

// failingReportEnv is an Env with a CAD config whose report store cannot be opened: NewReport returns err.
func failingReportEnv(err error, stdout, stderr io.Writer, opts ...envOption) cli.Env {
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, err },
	}
	for _, opt := range opts {
		opt(&env)
	}
	return env
}

// withClock gives the Env now as its clock.
func withClock(now func() time.Time) envOption { return func(env *cli.Env) { env.Now = now } }

// atTime pins the Env clock to at.
func atTime(at time.Time) envOption { return withClock(func() time.Time { return at }) }

// withLoader replaces the Env's config loader.
func withLoader(load cli.ConfigLoader) envOption { return func(env *cli.Env) { env.LoadConfig = load } }

// withConfig makes the Env's config loader answer cfg.
func withConfig(cfg config.Config) envOption {
	return withLoader(func(string) (config.Config, error) { return cfg, nil })
}

// leftOutWarning is the warning command prints for an account Quicken's reports leave out.
func leftOutWarning(command, name string) string {
	return "account \"" + name + "\" is not used in reports in Quicken, so " + command + " leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
}

// linkedTrackingWarning is the warning command prints for an account on linked account tracking.
func linkedTrackingWarning(command, name string) string {
	return "account \"" + name + "\" uses linked account tracking in Quicken, so " + command + " leaves it out, as Quicken's reports do"
}
