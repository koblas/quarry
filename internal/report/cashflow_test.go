package report_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cashFlowOf(t *testing.T, flow store.CashFlow, req report.CashFlowRequest) report.CashFlow {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{cashFlow: flow}))

	result, err := srv.CashFlow(t.Context(), req)

	require.NoError(t, err)
	return result
}

func monthlyRequest(since, until time.Time) report.CashFlowRequest {
	return report.CashFlowRequest{Window: store.Window{Since: since, Until: until}, By: store.CashFlowByMonth}
}

func yearlyRequest(since, until time.Time) report.CashFlowRequest {
	return report.CashFlowRequest{Window: store.Window{Since: since, Until: until}, By: store.CashFlowByYear}
}

// cadTotal is a cash flow with a CAD total, so a series has a currency to fill.
func cadTotal() store.CashFlow {
	return store.CashFlow{Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 1}}}
}

func cashFlowPartials(result report.CashFlow) []bool {
	flags := make([]bool, len(result.Rows))
	for i, r := range result.Rows {
		flags[i] = r.Partial
	}
	return flags
}

func cashFlowLabels(result report.CashFlow) []string {
	labels := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		labels[i] = r.Period
	}
	return labels
}

func Test_cashflow_fills_a_month_with_no_income_or_spending_as_zero_without_a_rate(t *testing.T) {
	flow := store.CashFlow{
		Rows: []store.CashFlowRow{
			{Period: "2026-01", Currency: "CAD", Income: 1000, Spent: 400, Net: 600, SavingsRatePct: new(60.0)},
			{Period: "2026-03", Currency: "CAD", Income: 500, Spent: 500},
		},
		Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 1500, Spent: 900, Net: 600, SavingsRatePct: new(40.0)}},
	}

	result := cashFlowOf(t, flow, monthlyRequest(day(2026, time.January, 1), day(2026, time.March, 31)))

	assert.Equal(t, []report.CashFlowRow{
		{CashFlowRow: flow.Rows[0]},
		{Period: "2026-02", Currency: "CAD"},
		{CashFlowRow: flow.Rows[1]},
	}, result.Rows)
}

func Test_cashflow_fills_every_period_for_each_currency_in_the_totals_order(t *testing.T) {
	flow := store.CashFlow{
		Rows: []store.CashFlowRow{{Period: "2026-01", Currency: "USD", Income: 100, Net: 100, SavingsRatePct: new(100.0)}},
		Totals: []store.CashFlowTotal{
			{Currency: "CAD", Income: 900},
			{Currency: "USD", Income: 100},
		},
	}

	result := cashFlowOf(t, flow, monthlyRequest(day(2026, time.January, 1), day(2026, time.February, 28)))

	got := make([][2]string, len(result.Rows))
	for i, r := range result.Rows {
		got[i] = [2]string{r.Period, r.Currency}
	}
	assert.Equal(t, [][2]string{{"2026-01", "CAD"}, {"2026-01", "USD"}, {"2026-02", "CAD"}, {"2026-02", "USD"}}, got)
	assert.Equal(t, int64(100), result.Rows[1].Income)
}

func Test_cashflow_by_year_lists_one_row_per_calendar_year_the_window_touches(t *testing.T) {
	flow := store.CashFlow{
		Rows:   []store.CashFlowRow{{Period: "2020", Currency: "CAD", Income: 800}, {Period: "2022", Currency: "CAD", Income: 200}},
		Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 1000}},
	}

	result := cashFlowOf(t, flow, yearlyRequest(day(2020, time.January, 1), day(2022, time.December, 31)))

	assert.Equal(t, []string{"2020", "2021", "2022"}, cashFlowLabels(result))
	assert.Equal(t, int64(0), result.Rows[1].Income)
}

func Test_cashflow_by_month_runs_the_series_across_a_year_end(t *testing.T) {
	result := cashFlowOf(t, cadTotal(), monthlyRequest(day(2025, time.November, 10), day(2026, time.February, 10)))

	assert.Equal(t, []string{"2025-11", "2025-12", "2026-01", "2026-02"}, cashFlowLabels(result))
}

func Test_cashflow_lists_no_periods_when_the_window_holds_no_income_or_spending(t *testing.T) {
	result := cashFlowOf(t, store.CashFlow{}, monthlyRequest(day(2026, time.January, 1), day(2026, time.March, 31)))

	assert.Empty(t, result.Rows)
	assert.True(t, result.Empty())
}

func Test_cashflow_is_not_empty_when_a_currency_nets_to_zero(t *testing.T) {
	flow := store.CashFlow{Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 500, Spent: 500}}}

	result := cashFlowOf(t, flow, monthlyRequest(day(2026, time.January, 1), day(2026, time.January, 31)))

	assert.False(t, result.Empty())
}

func Test_cashflow_reports_the_totals_and_the_transaction_range_the_store_found(t *testing.T) {
	span := store.TransactionRange{First: day(2003, time.January, 4), Last: day(2025, time.December, 31)}
	totals := []store.CashFlowTotal{{Currency: "CAD", Income: 900, Spent: 300, Net: 600, SavingsRatePct: new(66.7)}}

	result := cashFlowOf(t, store.CashFlow{Totals: totals, Transactions: span}, monthlyRequest(day(2026, time.March, 1), day(2026, time.March, 31)))

	assert.Equal(t, totals, result.Totals)
	assert.Equal(t, span, result.Transactions)
}

func Test_cashflow_passes_the_window_period_and_resolved_account_ids_to_the_store(t *testing.T) {
	var got store.CashFlowParams
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chequing, savings), gotCashFlow: &got}))
	req := yearlyRequest(day(2020, time.January, 1), day(2022, time.December, 31))
	req.Accounts = []string{"savings", "acct-100"}

	result, err := srv.CashFlow(t.Context(), req)

	require.NoError(t, err)
	assert.Equal(t, store.CashFlowParams{Window: req.Window, By: store.CashFlowByYear, AccountIDs: []string{"acct-200", "acct-100"}}, got)
	assert.Equal(t, []store.Account{savings, chequing}, result.Accounts)
	assert.Equal(t, req.Window, result.Window)
	assert.Equal(t, store.CashFlowByYear, result.By)
}

func Test_cashflow_does_not_read_accounts_when_none_is_named(t *testing.T) {
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{accountsReads: &reads}))

	result, err := srv.CashFlow(t.Context(), report.CashFlowRequest{})

	require.NoError(t, err)
	assert.Zero(t, reads)
	assert.Empty(t, result.Accounts)
}

func Test_cashflow_refuses_an_unknown_account_without_asking_the_store_for_cash_flow(t *testing.T) {
	var got store.CashFlowParams
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chequing), gotCashFlow: &got}))

	_, err := srv.CashFlow(t.Context(), report.CashFlowRequest{Accounts: []string{"Chequeing"}})

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, `no account named "Chequeing"; run quarry accounts --all to list them`, refusal.Error())
	assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
	assert.Equal(t, "Chequeing", refusal.Arg)
	assert.Equal(t, store.CashFlowParams{}, got)
}

func Test_cashflow_refuses_an_ambiguous_account_name_listing_its_ids_sorted(t *testing.T) {
	list := accountsOf(store.Account{ID: "acct-977", Name: "Visa"}, store.Account{ID: "acct-812", Name: "visa"})
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list}))

	_, err := srv.CashFlow(t.Context(), report.CashFlowRequest{Accounts: []string{"VISA"}})

	assert.EqualError(t, err, `2 accounts are named "VISA"; pass one of their ids instead: acct-812, acct-977`)
}

func Test_cashflow_reads_the_requested_currency_and_returns_it_with_the_result(t *testing.T) {
	var got store.CashFlowParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCashFlow: &got}))

	result, err := srv.CashFlow(t.Context(), report.CashFlowRequest{Currency: money.USD})

	require.NoError(t, err)
	assert.Equal(t, money.USD, got.Currency)
	assert.Equal(t, money.USD, result.Currency)
}

func Test_cashflow_marks_a_period_partial_only_when_the_window_cuts_it(t *testing.T) {
	cases := []struct {
		name string
		req  report.CashFlowRequest
		want []bool
	}{
		{name: "by month since on the first day", req: monthlyRequest(day(2026, time.January, 1), day(2026, time.February, 28)), want: []bool{false, false}},
		{name: "by month since on the second day", req: monthlyRequest(day(2026, time.January, 2), day(2026, time.February, 28)), want: []bool{true, false}},
		{name: "by month until the day before the last", req: monthlyRequest(day(2026, time.January, 1), day(2026, time.February, 27)), want: []bool{false, true}},
		{name: "by year the first and last year only", req: yearlyRequest(day(2020, time.March, 1), day(2022, time.June, 30)), want: []bool{true, false, true}},
		{name: "by year a window of whole years", req: yearlyRequest(day(2020, time.January, 1), day(2021, time.December, 31)), want: []bool{false, false}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := cashFlowOf(t, cadTotal(), c.req)

			assert.Equal(t, c.want, cashFlowPartials(result))
		})
	}
}

func Test_cashflow_refuses_when_a_read_fails_to_open_the_store(t *testing.T) {
	cases := []struct {
		name string
		req  report.CashFlowRequest
	}{
		{name: "the accounts read", req: report.CashFlowRequest{Accounts: []string{"Chequing"}}},
		{name: "the cash flow read", req: report.CashFlowRequest{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
			srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

			_, err := srv.CashFlow(t.Context(), c.req)

			assert.EqualError(t, err, missingStoreRefusal)
		})
	}
}

func Test_cashflow_reports_an_interrupt_during_a_read(t *testing.T) {
	cases := []struct {
		name string
		req  report.CashFlowRequest
	}{
		{name: "the cash flow read", req: report.CashFlowRequest{}},
		{name: "the accounts read", req: report.CashFlowRequest{Accounts: []string{"Chequing"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

			_, err := srv.CashFlow(ctx, c.req)

			assert.EqualError(t, err, "cashflow interrupted")
		})
	}
}
