package report_test

import (
	"cmp"
	"context"
	"errors"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
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

// Investment fixtures shared by the ACB tests.

const acbMillion = 1_000_000

var acbToday = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

// acbClassification lists acct-1, acct-2 and acct-3 non-registered and acct-9 registered.
func acbClassification() report.Classification {
	return report.Classification{
		Registered:    []string{"acct-9"},
		NonRegistered: []string{"acct-1", "acct-2", "acct-3"},
	}
}

func acbAccounts() []store.Account {
	return []store.Account{
		{ID: "acct-1", Name: "Margin", Type: store.AccountTypeBrokerage, Currency: "CAD"},
		{ID: "acct-2", Name: "Old margin", Type: store.AccountTypeBrokerage, Currency: "CAD", Closed: true},
		{ID: "acct-3", Name: "US margin", Type: store.AccountTypeBrokerage, Currency: "USD"},
		{ID: "acct-9", Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD"},
	}
}

func acbSecurity(id, name, currency string) store.Security {
	return store.Security{ID: id, Name: name, Ticker: &name, Currency: &currency}
}

// acbTx is one investment transaction with shares in millionths and amount in cents, no commission.
func acbTx(t *testing.T, sourceID int64, account, security, date, action, currency string, shares, amount int64) store.InvestmentTransaction {
	t.Helper()
	return store.InvestmentTransaction{
		ID:         "itxn-" + date + "-" + account + "-" + security + "-" + action,
		SourceID:   sourceID,
		AccountID:  account,
		SecurityID: &security,
		Date:       dateOf(t, date),
		Action:     action,
		Shares:     &shares,
		Amount:     amount,
		Currency:   currency,
	}
}

func acbRate(t *testing.T, date string, usdcad money.Rate) store.Rate {
	t.Helper()
	return store.Rate{Date: dateOf(t, date), USDCAD: usdcad, Series: store.SeriesCurrent}
}

type acbSaleRow struct {
	Date                                string
	Security                            string
	Shares                              string
	Proceeds, Outlays, ACBRemoved, Gain int64
}

type acbYearRow struct {
	Year                                int
	Sales                               []acbSaleRow
	Proceeds, Outlays, ACBRemoved, Gain int64
	ReturnOfCapitalGain                 int64
}

type acbPositionRow struct {
	Name   string
	Shares string
	ACB    int64
}

func acbYearRows(result report.ACB) []acbYearRow {
	rows := make([]acbYearRow, 0, len(result.Years))
	for _, year := range result.Years {
		row := acbYearRow{
			Year: year.Year, Proceeds: year.Proceeds, Outlays: year.Outlays, ACBRemoved: year.ACBRemoved, Gain: year.Gain,
			ReturnOfCapitalGain: year.ReturnOfCapitalGain,
		}
		for _, sale := range year.Sales {
			row.Sales = append(row.Sales, acbSaleRow{
				Date:       sale.Date.Format(time.DateOnly),
				Security:   sale.SecurityID,
				Shares:     sale.Shares.RatString(),
				Proceeds:   sale.Proceeds,
				Outlays:    sale.Outlays,
				ACBRemoved: sale.ACBRemoved,
				Gain:       sale.Gain,
			})
		}
		rows = append(rows, row)
	}
	return rows
}

func acbPositionRows(result report.ACB) []acbPositionRow {
	rows := make([]acbPositionRow, 0, len(result.Securities))
	for _, position := range result.Securities {
		rows = append(rows, acbPositionRow{Name: position.Security.Name, Shares: position.Shares.RatString(), ACB: position.ACB})
	}
	return rows
}

// acbSplitTx is a sec-1 split of newShares for oldShares, both in millionths.
func acbSplitTx(t *testing.T, sourceID int64, account, date string, newShares, oldShares int64) store.InvestmentTransaction {
	t.Helper()
	split := acbTx(t, sourceID, account, "sec-1", date, store.ActionSplit, "CAD", 0, 0)
	split.Shares, split.SplitNewShares, split.SplitOldShares = nil, &newShares, &oldShares
	return split
}

// acbWalkOf walks txs over acct-1, acct-2 (closed) and acct-3 non-registered, acct-9 registered and acct-7,
// a chequing account, in neither list, with securities sec-1 XEQT and sec-2 VTI.
func acbWalkOf(t *testing.T, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD"), acbSecurity("sec-2", "VTI", "CAD")}, nil, txs...)
}

// acbWalkWith is acbWalkOf over the given securities and exchange rates, which are in date order.
func acbWalkWith(t *testing.T, securities []store.Security, rates []store.Rate, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkRequest(t, securities, rates, nil, txs...)
}

// acbWalkAdjusted is acbWalkOf with the adjustments, whose item numbers are their places in the list.
func acbWalkAdjusted(t *testing.T, adjustments []report.ACBAdjustment, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkRequest(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD"), acbSecurity("sec-2", "VTI", "CAD")}, nil, adjustments, txs...)
}

func acbWalkRequest(t *testing.T, securities []store.Security, rates []store.Rate, adjustments []report.ACBAdjustment, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	txs = slices.SortedStableFunc(slices.Values(txs), func(a, b store.InvestmentTransaction) int {
		return cmp.Or(a.Date.Compare(b.Date), cmp.Compare(a.SourceID, b.SourceID))
	})
	unlisted := store.Account{ID: "acct-7", Name: "Cash margin", Type: "chequing", Currency: "CAD"}
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:     append(acbAccounts(), unlisted),
		Securities:   securities,
		Transactions: txs,
		Rates:        rates,
	}}))

	got, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday, Adjustments: adjustments})

	require.NoError(t, err)
	return got
}

func acbSaleRows(result report.ACB) []acbSaleRow {
	var rows []acbSaleRow
	for _, year := range acbYearRows(result) {
		rows = append(rows, year.Sales...)
	}
	return rows
}

// acbFlags is each sale's possible-superficial-loss mark, in the order the years list the sales.
func acbFlags(result report.ACB) []bool {
	var flags []bool
	for _, year := range result.Years {
		for _, sale := range year.Sales {
			flags = append(flags, sale.PossibleSuperficialLoss)
		}
	}
	return flags
}

// selectHistory is securities bought in non-registered acct-1 (shared tickers and names, no or empty ticker), sec-3
// and sec-13 only in registered acct-9, sec-5 there the day after today, sec-9 only in acct-7, in neither list, sec-4 never.
func selectHistory(t *testing.T) store.InvestmentHistory {
	t.Helper()
	empty := ""
	cad := "CAD"
	blank := store.Security{ID: "sec-8", Name: "Blank", Ticker: &empty}
	tickerless := store.Security{ID: "sec-10", Name: "No Ticker", Currency: &cad}
	return store.InvestmentHistory{
		Accounts: append(acbAccounts(), store.Account{ID: "acct-7", Name: "Cash margin", Type: "chequing", Currency: "CAD"}),
		Securities: []store.Security{
			selectSecurity("sec-1", "Acme Corp", "ACME"),
			selectSecurity("sec-2", "Beta Inc", "BETA"),
			selectSecurity("sec-3", "Maple", "MPL"),
			selectSecurity("sec-4", "Adjusted", "ADJ"),
			selectSecurity("sec-5", "Future", "FUT"),
			selectSecurity("sec-6", "Acme Preferred", "acme"),
			selectSecurity("sec-7", "sec-2", "SEC7"),
			blank,
			selectSecurity("sec-9", "Cash only", "CSH"),
			tickerless,
			selectSecurity("sec-11", "beta Fund", "BFD"),
			selectSecurity("sec-b", "Twin", "TWB"),
			selectSecurity("sec-a", "Twin", "TWA"),
			selectSecurity("sec-13", "Today Fund", "TDY"),
		},
		Transactions: []store.InvestmentTransaction{
			acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 2, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 3, "acct-9", "sec-3", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 4, "acct-9", "sec-5", "2026-10-06", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 5, "acct-1", "sec-6", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 6, "acct-1", "sec-7", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 7, "acct-1", "sec-8", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 8, "acct-7", "sec-9", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 9, "acct-1", "sec-10", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 10, "acct-1", "sec-11", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 11, "acct-1", "sec-b", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 12, "acct-1", "sec-a", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 13, "acct-9", "sec-13", acbToday.Format(time.DateOnly), store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		},
	}
}

func selectACB(t *testing.T, history store.InvestmentHistory, selectors ...string) (report.ACB, error) {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{history: history}))

	return srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday, Securities: selectors})
}

func securityIDs(a report.ACB) []string {
	ids := make([]string, 0, len(a.Securities))
	for _, s := range a.Securities {
		ids = append(ids, s.Security.ID)
	}

	return ids
}

// Finding, anomaly and account fixtures shared across files.

// since is the window from date through recurringNow's day.
func since(t *testing.T, date string) store.Window {
	t.Helper()
	return store.Window{Since: dateOf(t, date), Until: thisYear.Until}
}

// earlierCharges is count charges of amount cents, 30 days apart from 2025-06-01.
func earlierCharges(t *testing.T, count int, amount int64, opts ...chargeOpt) []store.Charge {
	t.Helper()
	return chargesOn(t, everyDays(t, "2025-06-01", 30, count), append([]chargeOpt{ofAmount(amount)}, opts...)...)
}

func amountsOf(anomalies []report.Anomaly) []int64 {
	amounts := make([]int64, 0, len(anomalies))
	for _, a := range anomalies {
		amounts = append(amounts, a.Amount)
	}
	return amounts
}

// dated is an open finding of typ with one item per date.
func dated(id string, typ finding.Type, dates ...time.Time) store.Finding {
	f := store.Finding{ID: id, Type: typ}
	for _, d := range dates {
		f.Items = append(f.Items, store.FindingItem{Date: d})
	}
	return f
}

func idsOf(g report.FindingsGroup) []string {
	ids := make([]string, len(g.Findings))
	for i, f := range g.Findings {
		ids[i] = f.ID
	}
	return ids
}

func listIDs(listing report.FindingsListing) map[finding.Type][]string {
	ids := map[finding.Type][]string{}
	for _, g := range listing.Groups {
		ids[g.Type] = idsOf(g)
	}
	return ids
}

var (
	march1  = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	march2  = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	march3  = time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	fixedAt = time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)
)

func typedRow(accountType, currency string, balance int64, cad *big.Int) store.NetWorthRow {
	return store.NetWorthRow{Date: netWorthDay, Type: accountType, Currency: currency, Balance: big.NewInt(balance), BalanceCAD: cad}
}

const unclassifiedOne = "unclassified-account:acct-1"

// accountsFindings lists the findings of req over accounts and stored findings.
func accountsFindings(t *testing.T, req report.FindingsRequest, accounts []store.Account, stored ...store.Finding) report.FindingsListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: stored, Accounts: accounts}}))
	got, err := srv.Findings(t.Context(), req)
	require.NoError(t, err)
	return got
}

func brokerage(id, name string) store.Account {
	return store.Account{ID: id, Name: name, Type: store.AccountTypeBrokerage, Currency: "CAD"}
}

const (
	refusalHome = "/Users/dave"
	storePath   = "/Users/dave/Library/Application Support/quarry/quarry.duckdb"
)

var errDiskRead = errors.New("read store status: disk read failed")
