package report_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

// missingStoreRefusal is the refusal text for a store that was never built, as WithHome(refusalHome) words it.
const missingStoreRefusal = "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it"

// fakeStore answers each read with a canned result or fault; Query returns at most maxRows of rows, and the
// got*/*Reads pointers, when set, record what each read was given or how often it was called.
type fakeStore struct {
	status     store.Status
	accounts   store.AccountList
	spending   store.Spending
	cashFlow   store.CashFlow
	charges    store.Charges
	findings   store.FindingList
	holdings   store.Holdings
	netWorth   store.NetWorth
	history    store.InvestmentHistory
	search     store.Search
	schema     store.Schema
	rows       [][]store.QueryValue
	gotMaxRows *int

	gotSpending   *store.SpendingParams
	gotCashFlow   *store.CashFlowParams
	gotCharges    *store.ChargeParams
	gotSearch     *store.SearchParams
	gotHoldings   *store.HoldingsParams
	gotNetWorth   *store.NetWorthParams
	gotSummary    *store.SummaryParams
	accountsReads *int
	holdingsReads *int
	netWorthReads *int
	summaryReads  *int
	historyReads  *int
	chargesReads  *int
	schemaReads   *int
	spendingReads *int
	cashFlowReads *int
	err           error
	// chargesErr, when set, is what Charges fails with instead of err.
	chargesErr error
	// summary, when set, is what Summary returns instead of an answer assembled from the fields above.
	summary *store.Summary
}

func (f fakeStore) Status(context.Context) (store.Status, error) { return f.status, f.err }

func (f fakeStore) Schema(context.Context) (store.Schema, error) {
	if f.schemaReads != nil {
		*f.schemaReads++
	}
	return f.schema, f.err
}

func (f fakeStore) Accounts(context.Context) (store.AccountList, error) {
	if f.accountsReads != nil {
		*f.accountsReads++
	}
	return f.accounts, f.err
}

func (f fakeStore) Spending(_ context.Context, params store.SpendingParams) (store.Spending, error) {
	if f.gotSpending != nil {
		*f.gotSpending = params
	}
	if f.spendingReads != nil {
		*f.spendingReads++
	}
	return f.spending, f.err
}

func (f fakeStore) CashFlow(_ context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	if f.gotCashFlow != nil {
		*f.gotCashFlow = params
	}
	if f.cashFlowReads != nil {
		*f.cashFlowReads++
	}
	return f.cashFlow, f.err
}

func (f fakeStore) Charges(_ context.Context, params store.ChargeParams) (store.Charges, error) {
	if f.gotCharges != nil {
		*f.gotCharges = params
	}
	if f.chargesReads != nil {
		*f.chargesReads++
	}
	if f.chargesErr != nil {
		return store.Charges{}, f.chargesErr
	}
	return f.charges, f.err
}

func (f fakeStore) Search(_ context.Context, params store.SearchParams) (store.Search, error) {
	if f.gotSearch != nil {
		*f.gotSearch = params
	}
	return f.search, f.err
}

func (f fakeStore) Holdings(_ context.Context, params store.HoldingsParams) (store.Holdings, error) {
	if f.gotHoldings != nil {
		*f.gotHoldings = params
	}
	if f.holdingsReads != nil {
		*f.holdingsReads++
	}
	return f.holdings, f.err
}

func (f fakeStore) Findings(context.Context) (store.FindingList, error) { return f.findings, f.err }

func (f fakeStore) Query(_ context.Context, _ string, maxRows int) (store.QueryResult, error) {
	if f.gotMaxRows != nil {
		*f.gotMaxRows = maxRows
	}
	rows := f.rows
	if maxRows > 0 {
		rows = rows[:min(maxRows, len(rows))]
	}
	return store.QueryResult{Rows: rows}, f.err
}

func (f fakeStore) NetWorth(_ context.Context, params store.NetWorthParams) (store.NetWorth, error) {
	if f.gotNetWorth != nil {
		*f.gotNetWorth = params
	}
	if f.netWorthReads != nil {
		*f.netWorthReads++
	}
	return f.netWorth, f.err
}

// Summary answers with the status, the charges dated through params.Through and the net worth the fake holds.
func (f fakeStore) Summary(_ context.Context, params store.SummaryParams) (store.Summary, error) {
	if f.gotSummary != nil {
		*f.gotSummary = params
	}
	if f.summaryReads != nil {
		*f.summaryReads++
	}
	if f.summary != nil {
		return *f.summary, f.err
	}
	charges := f.charges
	charges.Rows = slices.DeleteFunc(slices.Clone(charges.Rows), func(c store.Charge) bool { return c.Date.After(params.Through) })
	return store.Summary{Status: f.status, Charges: charges, NetWorth: f.netWorth}, f.err
}

func (f fakeStore) InvestmentHistory(context.Context) (store.InvestmentHistory, error) {
	if f.historyReads != nil {
		*f.historyReads++
	}
	return f.history, f.err
}

// recurringNow is the clock every recurring test reads: 2026-09-29 noon UTC.
var recurringNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// chargeOpt changes a charge chargeOn built.
type chargeOpt func(*store.Charge)

func paidTo(id, name string) chargeOpt {
	return func(c *store.Charge) { c.PayeeID, c.Payee = &id, &name }
}

func unpaid() chargeOpt {
	return func(c *store.Charge) { c.PayeeID, c.Payee = nil, nil }
}

func billedIn(currency string) chargeOpt { return func(c *store.Charge) { c.Currency = currency } }

func ofAmount(cents int64) chargeOpt { return func(c *store.Charge) { c.Amount = cents } }

func onAccount(id, name string) chargeOpt {
	return func(c *store.Charge) {
		c.Account = store.Account{ID: id, Name: name, Currency: c.Currency, Active: true}
	}
}

func inCategory(id, path string) chargeOpt {
	return func(c *store.Charge) { c.Category = &store.ChargeCategory{ID: id, Path: path} }
}

// chargeOn is a 12.00 CAD charge by Gym on date (YYYY-MM-DD), numbered sourceID, changed by opts.
func chargeOn(t *testing.T, sourceID int64, date string, opts ...chargeOpt) store.Charge {
	t.Helper()
	day, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	c := store.Charge{
		SourceID: sourceID,
		Date:     day,
		Account:  store.Account{ID: "acct-cad", Name: "Chequing", Currency: "CAD", Active: true},
		PayeeID:  new("payee-gym"),
		Payee:    new("Gym"),
		Currency: "CAD",
		Amount:   1200,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// everyDays is count dates gapDays apart from first (YYYY-MM-DD).
func everyDays(t *testing.T, first string, gapDays, count int) []string {
	t.Helper()
	start, err := time.Parse(time.DateOnly, first)
	require.NoError(t, err)
	dates := make([]string, count)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, gapDays*i).Format(time.DateOnly)
	}
	return dates
}

// chargesOn is a charge on each date, built with opts.
func chargesOn(t *testing.T, dates []string, opts ...chargeOpt) []store.Charge {
	t.Helper()
	charges := make([]store.Charge, len(dates))
	for i, date := range dates {
		charges[i] = chargeOn(t, 0, date, opts...)
	}
	return charges
}

// allTime is a window from 2000 through recurringNow's day.
var allTime = store.Window{
	Since: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
}

// recurringOf reads the series of every charge in groups over allTime.
func recurringOf(t *testing.T, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringIn(t, allTime, groups...)
}

// recurringIn reads the series of every charge in groups, merged oldest first and numbered in that order,
// listing those that ran in window.
func recurringIn(t *testing.T, window store.Window, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringAt(t, recurringNow, window, groups...)
}

// recurringAt is recurringIn read at now.
func recurringAt(t *testing.T, now time.Time, window store.Window, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringRead(t, report.RecurringRequest{Window: window, Now: now}, time.Time{}, groups...)
}

// mergedByDate is the charges of groups in one slice, oldest first (ties keep the order given) and numbered
// 1, 2, ... in that order; the groups themselves are left as they were.
func mergedByDate(groups ...[]store.Charge) []store.Charge {
	merged := slices.Concat(groups...)
	slices.SortStableFunc(merged, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	for i := range merged {
		merged[i].SourceID = int64(i + 1)
	}
	return merged
}

// recurringRead answers req from the merged charges of groups (oldest first, numbered in that order)
// on a store whose first exchange rate is dated firstRate.
func recurringRead(t *testing.T, req report.RecurringRequest, firstRate time.Time, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: mergedByDate(groups...), FirstRate: firstRate}}))

	result, err := srv.Recurring(t.Context(), req)

	require.NoError(t, err)
	return result
}

// monthlyEndingOn is count charges 30 days apart, the last on date (YYYY-MM-DD), built with opts.
func monthlyEndingOn(t *testing.T, date string, count int, opts ...chargeOpt) []store.Charge {
	t.Helper()
	return everyDaysEndingOn(t, date, 30, count, opts...)
}

// everyDaysEndingOn is count charges gapDays apart, the last on date (YYYY-MM-DD), built with opts.
func everyDaysEndingOn(t *testing.T, date string, gapDays, count int, opts ...chargeOpt) []store.Charge {
	t.Helper()
	last, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	first := last.AddDate(0, 0, -gapDays*(count-1))
	return chargesOn(t, everyDays(t, first.Format(time.DateOnly), gapDays, count), opts...)
}

// payeesOf lists the payee of each series in result, in order.
func payeesOf(result report.Recurring) []string {
	payees := make([]string, len(result.Series))
	for i, s := range result.Series {
		payees[i] = s.Payee
	}
	return payees
}

func dateOf(t *testing.T, date string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	return parsed
}

var (
	chqAccount   = store.Account{ID: "acct-chq", Name: "Chequing"}
	visaAccount  = store.Account{ID: "acct-visa", Name: "Visa"}
	savAccount   = store.Account{ID: "acct-sav", Name: "Savings"}
	midAccount   = store.Account{ID: "acct-mid", Name: "Mid"}
	namedRefusal = `no account named "Nope"; run quarry accounts --all to list them`
)

// onAccountOf puts every charge in charges on account.
func onAccountOf(account store.Account, charges []store.Charge) []store.Charge {
	for i := range charges {
		charges[i].Account = account
	}
	return charges
}

// firstRateDay is the date of the first exchange rate on the store behind recurringListedIn.
var firstRateDay = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

func inCAD(cents int64) chargeOpt { return func(c *store.Charge) { c.AmountCAD = &cents } }

func inUSD(cents int64) chargeOpt { return func(c *store.Charge) { c.AmountUSD = &cents } }

// summaryNow is the clock the summary tests read: 2026-10-06 noon UTC, after September 2026 ended.
var summaryNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

var (
	chequing = store.Account{ID: "acct-100", Name: "Chequing"}
	savings  = store.Account{ID: "acct-200", Name: "Savings"}
	oldCard  = store.Account{ID: "acct-300", Name: "Old Card", Closed: true, NotInReports: true}
)

func accountsOf(accounts ...store.Account) store.AccountList {
	list := store.AccountList{}
	for _, a := range accounts {
		list.Accounts = append(list.Accounts, store.AccountBalance{Account: a})
	}
	return list
}

// windowNow is 2026-09-29 in a zone where that evening is already 2026-09-30 in UTC.
var windowNow = time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60))

func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

// edt is a zone whose midnight falls on the previous day in UTC.
var edt = time.FixedZone("EDT", -4*60*60)
