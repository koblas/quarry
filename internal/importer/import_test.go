package importer_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_fails_when_the_snapshot_path_does_not_exist(t *testing.T) {
	t.Parallel()
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: "/no/such/snapshot.sqlite"})

	require.Error(t, err)
}

// Covers every table in one pass: an account with an institution, a
// two-way split transaction, a category, a payee and a tag on one split.
func Test_import_builds_every_table_from_a_v9_snapshot(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	bankPK := b.Institution(v9fixture.InstitutionRow{Name: "Big Bank"})
	acctPK := b.Account(v9fixture.AccountRow{
		Name: "Chequing", Type: "CHECKING", Currency: "CAD", Institution: bankPK, Active: true,
	})
	payeePK := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})

	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{
		Account: acctPK, Amount: "12.34", PostedDate: &posted, Payee: payeePK,
	})
	entry1PK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "7.00", CategoryTag: catPK})
	entry2PK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "5.34"})
	b.LinkUserTag(entry1PK, tagPK)

	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{Path: "/store/quarry.duckdb"}
	srv := importer.NewServer(importer.WithStore(fake))

	result, err := srv.Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, fake.Path, result.Path)
	assert.Equal(t, store.Counts{
		Accounts: 1, Categories: 1, Payees: 1, Tags: 1, Transactions: 1, Splits: 2, SplitTags: 1,
	}, result.Counts)

	acctID := fmt.Sprintf("acct-%d", acctPK)
	catID := fmt.Sprintf("cat-%d", catPK)
	payeeID := fmt.Sprintf("payee-%d", payeePK)
	tagID := fmt.Sprintf("tag-%d", tagPK)
	txnID := fmt.Sprintf("txn-%d", txnPK)
	split1ID := fmt.Sprintf("split-%d", entry1PK)
	split2ID := fmt.Sprintf("split-%d", entry2PK)

	assert.Equal(t, []store.Account{{
		ID: acctID, SourceID: acctPK, Name: "Chequing", Type: "chequing", Currency: "CAD",
		Institution: new("Big Bank"), Closed: false, Active: true,
	}}, fake.Rows.Accounts)

	assert.Equal(t, []store.Category{{
		ID: catID, SourceID: catPK, Name: "Groceries", FullPath: "Groceries", Kind: "expense", Hidden: false,
	}}, fake.Rows.Categories)

	assert.Equal(t, []store.Payee{{ID: payeeID, SourceID: payeePK, Name: "Coffee Shop"}}, fake.Rows.Payees)

	assert.Equal(t, []store.Tag{{ID: tagID, SourceID: tagPK, Name: "Reimbursable"}}, fake.Rows.Tags)

	assert.Equal(t, []store.Transaction{{
		ID: txnID, SourceID: txnPK, AccountID: acctID, Date: posted, PostedDate: &posted,
		PayeeID: new(payeeID), Amount: 1234, Currency: "CAD", Status: "uncleared",
	}}, fake.Rows.Transactions)

	assert.ElementsMatch(t, []store.Split{
		{ID: split1ID, SourceID: entry1PK, TransactionID: txnID, CategoryID: new(catID), Amount: 700},
		{ID: split2ID, SourceID: entry2PK, TransactionID: txnID, Amount: 534},
	}, fake.Rows.Splits)

	assert.Equal(t, []store.SplitTag{{SplitID: split1ID, TagID: tagID}}, fake.Rows.SplitTags)
}

// Ids must derive only from each row's own Z_PK, never from anything a
// second build could compute differently for the same row.
func Test_import_twice_from_the_same_snapshot_keeps_every_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	fake1 := &fakeStore{}
	_, err := importer.NewServer(importer.WithStore(fake1)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})
	require.NoError(t, err)

	fake2 := &fakeStore{}
	_, err = importer.NewServer(importer.WithStore(fake2)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})
	require.NoError(t, err)

	wantAcctID := fmt.Sprintf("acct-%d", acctPK)
	wantTxnID := fmt.Sprintf("txn-%d", txnPK)
	require.Len(t, fake1.Rows.Accounts, 1)
	require.Len(t, fake2.Rows.Accounts, 1)
	assert.Equal(t, wantAcctID, fake1.Rows.Accounts[0].ID)
	assert.Equal(t, wantAcctID, fake2.Rows.Accounts[0].ID)
	require.Len(t, fake1.Rows.Transactions, 1)
	require.Len(t, fake2.Rows.Transactions, 1)
	assert.Equal(t, wantTxnID, fake1.Rows.Transactions[0].ID)
	assert.Equal(t, wantTxnID, fake2.Rows.Transactions[0].ID)
}

// Each category keeps its own parent_id, full_path, kind and hidden flag,
// across income, expense and system kinds and a nested parent/child pair.
func Test_import_keeps_each_categorys_parent_path_kind_and_hidden(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	incomePK := b.Category(v9fixture.TagRow{Name: "Salary", Type: new(int64(2))})
	systemPK := b.Category(v9fixture.TagRow{Name: "Transfer", Type: new(int64(0))})
	parentPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	childPK := b.Category(v9fixture.TagRow{
		Name: "Groceries", Type: new(int64(1)), ParentCategory: parentPK, Hidden: true,
	})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	parentID := fmt.Sprintf("cat-%d", parentPK)
	assert.ElementsMatch(t, []store.Category{
		{ID: fmt.Sprintf("cat-%d", incomePK), SourceID: incomePK, Name: "Salary", FullPath: "Salary", Kind: "income"},
		{ID: fmt.Sprintf("cat-%d", systemPK), SourceID: systemPK, Name: "Transfer", FullPath: "Transfer", Kind: "system"},
		{ID: parentID, SourceID: parentPK, Name: "Food", FullPath: "Food", Kind: "expense"},
		{
			ID: fmt.Sprintf("cat-%d", childPK), SourceID: childPK, ParentID: new(parentID),
			Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense", Hidden: true,
		},
	}, fake.Rows.Categories)
}

func Test_import_hands_the_store_one_import_run_describing_the_build(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := chequingWithOneReconciledTxn(b, "100.00")
	day := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	transferLeg(b, chequingPK, "-5.00", 101, "Old Visa")
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	transferLeg(b, chequingPK, "-20.00", 201, "202")
	transferLeg(b, savingsPK, "15.00", 202, "201")
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	investmentWithEntry(b, v9fixture.TransactionRow{Type: new(int64(3)), Account: brokeragePK, Amount: "-40.00", PostedDate: &day})
	bundle := b.WriteBundle(t, t.TempDir())
	snap := store.SnapshotRef{Path: bundle.DataPath, SHA256: "9f86d081", SchemaFingerprint: "sha256:abc"}
	fake := &fakeStore{}
	before := time.Now().UTC()

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), snap)

	after := time.Now().UTC()
	require.NoError(t, err)
	require.Len(t, fake.Rows.ImportRuns, 1)
	run := fake.Rows.ImportRuns[0]
	assert.Equal(t, store.ImportRun{
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Snapshot: snap,
		Counts:          store.Counts{Accounts: 3, Transactions: 5, Splits: 5, Transfers: 2, InvestmentTransactions: 1},
		BalancesChecked: 1, TransfersOneSided: 1,
		BalancesNeverReconciled: 1, InvestmentAccounts: 1, TransfersPaired: 1, TransfersCrossCurrency: 1,
	}, run)
	assert.Equal(t, time.UTC, run.StartedAt.Location())
	assert.Equal(t, time.UTC, run.FinishedAt.Location())
	assert.False(t, run.StartedAt.Before(before))
	assert.False(t, run.FinishedAt.Before(run.StartedAt))
	assert.False(t, after.Before(run.FinishedAt))
}

func Test_import_counts_securities_and_prices_in_the_import_run(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay2, ClosingPrice: "13"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &priceDay1, ClosingPrice: "4"})

	fake, _ := importSecurities(t, b)

	require.Len(t, fake.Rows.ImportRuns, 1)
	assert.Equal(t, store.Counts{Accounts: 1, Securities: 2, Prices: 3}, fake.Rows.ImportRuns[0].Counts)
}

func Test_import_reports_the_history_fault_the_store_returns(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: "/store/quarry.duckdb"}
	fake := &fakeStore{historyFault: fault}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.Built)
	assert.Same(t, fault, result.HistoryFault)
}

func Test_import_reports_no_history_fault_when_the_store_returns_none(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.Built)
	assert.Nil(t, result.HistoryFault)
}

func Test_import_passes_the_carry_faults_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: "/store/quarry.duckdb", Reason: "its findings table repeats an id"}
	fake := &fakeStore{findingsFault: fault, unreadable: true}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Same(t, fault, result.FindingsFault)
	assert.True(t, result.StoreUnreadable)
}

func Test_import_passes_the_rates_fault_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: "/store/quarry.duckdb", Reason: "its fx_rates table repeats a date"}
	fake := &fakeStore{ratesFault: fault}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Same(t, fault, result.RatesFault)
}

func Test_import_returns_the_stores_findings_counts(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	counts := finding.Counts{Open: 4, New: 3}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{findings: counts})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, counts, result.Findings)
}

func Test_import_passes_the_finding_states_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	states := []finding.State{{ID: "uncategorized:payee-1", New: true}, {ID: "duplicate:txn-1:txn-2", Fixed: true, NewlyFixed: true}}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{states: states})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, states, result.FindingStates)
}

func Test_import_returns_the_accounts_it_wrote_to_the_store(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Accounts, 2)
	assert.Equal(t, fake.Rows.Accounts, result.Accounts)
}

func Test_import_returns_whether_the_stores_findings_were_carried(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{carried: true})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.FindingsCarried)
}

func Test_import_passes_the_rates_summary_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	rates := store.RatesSummary{
		First: time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Added: 7, FetchError: "unreachable",
	}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{rates: rates})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, rates, result.Rates)
}

func Test_import_returns_the_securities_and_investment_transactions_it_wrote(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: positionPK, Units: "0"})

	fake, result := importInvestments(t, b)

	require.Len(t, fake.Rows.Securities, 1)
	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, store.Investments{Securities: fake.Rows.Securities, Transactions: fake.Rows.InvestmentTransactions}, result.Investments)
}

var errShareCheckFault = errors.New("scratch database unavailable")

func oneShareMismatch() store.ShareCheck {
	return store.ShareCheck{
		Checked:    2,
		Mismatched: []store.ShareMismatch{{AccountID: "acct-1", SecurityID: "sec-1", Quarry: 1000000, Quicken: 2000000}},
	}
}

func Test_import_does_not_replace_the_store_when_share_counts_differ(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: oneShareMismatch()}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Zero(t, fake.replaceCalls)
	assert.False(t, result.Built)
	assert.Equal(t, 2, result.Validation.Shares.Checked)
	require.Len(t, result.Validation.Shares.Mismatched, 1)
	assert.Equal(t, "acct-1", result.Validation.Shares.Mismatched[0].AccountID)
}

func Test_import_replaces_the_store_and_records_the_holdings_checked_when_share_counts_match(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: 3}}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, 1, fake.replaceCalls)
	assert.Equal(t, 3, result.Validation.Shares.Checked)
	require.Len(t, fake.Rows.ImportRuns, 1)
	assert.Equal(t, 3, fake.Rows.ImportRuns[0].SharesChecked)
}

func Test_import_reports_share_counts_alongside_a_balance_failure(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: 4}}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Len(t, result.Validation.Balances.Mismatched, 1)
	assert.Equal(t, 4, result.Validation.Shares.Checked)
}

func Test_import_returns_the_share_check_error_without_replacing_the_store(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareErr: errShareCheckFault}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, errShareCheckFault)
	require.NotErrorIs(t, err, store.ErrValidationFailed)
	require.EqualError(t, err, "check share counts: scratch database unavailable")
	assert.Zero(t, fake.replaceCalls)
}

// faultingSource wraps a real Source, failing every QueryRows call whose
// query text contains match with err — one query point at a time.
type faultingSource struct {
	real  importer.Source
	match string
	err   error
}

func (f *faultingSource) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if strings.Contains(query, f.match) {
		return f.err
	}
	return f.real.QueryRows(ctx, query, args, row)
}

func (f *faultingSource) Close() error { return f.real.Close() }

// errSourceBoom and errStoreDiskFull are the faults the fakes below inject.
var (
	errSourceBoom    = errors.New("boom")
	errStoreDiskFull = errors.New("disk full")
)

func openerFailingOn(match string, err error) importer.SourceOpener {
	return func(ctx context.Context, path string) (importer.Source, error) {
		src, openErr := sqlite.OpenReadOnly(ctx, path)
		if openErr != nil {
			return nil, openErr
		}
		return &faultingSource{real: src, match: match, err: err}, nil
	}
}

// Import must propagate a failure from every source query it issues, not
// swallow it — one row per table the importer reads from.
func Test_import_propagates_a_fault_from_every_source_query(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true, LoanInterestCategory: catPK})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: catPK})
	b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: catPK})
	b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: catPK})
	b.ProductService(v9fixture.ProductServiceRow{Category: catPK})
	b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: catPK})
	b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: catPK})
	b.LinkUserTag(entryPK, tagPK)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "1.00"})
	securityPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: securityPK, QuoteDate: &posted, ClosingPrice: "12.5"})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: securityPK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &posted, Position: positionPK, Units: "1"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "1"})
	bundle := b.WriteBundle(t, t.TempDir())

	errBoom := errSourceBoom
	cases := []struct {
		name  string
		match string
		want  string
	}{
		{"Z_PRIMARYKEY", "Z_PRIMARYKEY", "resolve entities"},
		{"ZACCOUNT", "ZTYPENAME", "read accounts"},
		{"ZRECONCILERECORD", "ZRECONCILERECORD", "read reconcile records"},
		{"ZTAG categories", "ZPARENTCATEGORY", "read categories"},
		{"ZTAG tags", "COALESCE(ZNAME, '')\nFROM ZTAG", "read tags"},
		{"ZUSERPAYEE", "ZUSERPAYEE", "read payees"},
		{"ZTRANSACTION", "ZPOSTEDDATE", "read transactions"},
		{"ZCASHFLOWTRANSACTIONENTRY", "FROM ZCASHFLOWTRANSACTIONENTRY", "read splits"},
		{"Z_15USERTAGS", "Z_15USERTAGS", "read split tags"},
		{"ZBUDGETLINEITEM", "FROM ZBUDGETLINEITEM", "read budget line items"},
		{"ZLOANSPLITENTRY", "FROM ZLOANSPLITENTRY", "read loan split entries"},
		{"ZACCOUNT loan interest", "ZLOANINTERESTCATEGORY", "read loan interest categories"},
		{"ZQUICKFILLRULESPLITENTRY", "FROM ZQUICKFILLRULESPLITENTRY", "read quickfill rule split entries"},
		{"ZPRODUCTSERVICE", "FROM ZPRODUCTSERVICE", "read product and service categories"},
		{"ZCUSTOMERCREDITLINEITEM", "FROM ZCUSTOMERCREDITLINEITEM", "read customer credit line item categories"},
		{"ZSECURITY", "ZTICKER", "read securities"},
		{"ZSECURITYQUOTE", "ZCLOSINGPRICE", "read prices"},
		{"ZPOSITION", "FROM ZPOSITION", "read positions"},
		{"ZLOT", "FROM ZLOT", "read lots"},
		{"ZTRANSACTION investments", "ZUNITS", "read investment transactions"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := importer.NewServer(importer.WithStore(&fakeStore{}), importer.WithSourceOpener(openerFailingOn(c.match, errBoom)))

			_, err := srv.Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			require.ErrorIs(t, err, errBoom)
			require.ErrorContains(t, err, c.want)
		})
	}
}

// scanFaultingSource lets a matched query's real rows through but fails
// every scan() call on them.
type scanFaultingSource struct {
	real  importer.Source
	match string
	err   error
}

func (f *scanFaultingSource) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if strings.Contains(query, f.match) {
		return f.real.QueryRows(ctx, query, args, func(func(dest ...any) error) error {
			return row(func(...any) error { return f.err })
		})
	}
	return f.real.QueryRows(ctx, query, args, row)
}

func (f *scanFaultingSource) Close() error { return f.real.Close() }

func openerScanFailingOn(match string, err error) importer.SourceOpener {
	return func(ctx context.Context, path string) (importer.Source, error) {
		src, openErr := sqlite.OpenReadOnly(ctx, path)
		if openErr != nil {
			return nil, openErr
		}
		return &scanFaultingSource{real: src, match: match, err: err}, nil
	}
}

// Import must propagate a Scan failure on a row it already fetched, not
// just a failure to run the query itself — one row per table.
func Test_import_propagates_a_scan_fault_from_every_source_query(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true, LoanInterestCategory: catPK})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: catPK})
	b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: catPK})
	b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: catPK})
	b.ProductService(v9fixture.ProductServiceRow{Category: catPK})
	b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: catPK})
	b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: catPK})
	b.LinkUserTag(entryPK, tagPK)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "1.00"})
	securityPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: securityPK, QuoteDate: &posted, ClosingPrice: "12.5"})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: securityPK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &posted, Position: positionPK, Units: "1"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "1"})
	bundle := b.WriteBundle(t, t.TempDir())

	errBoom := errSourceBoom
	cases := []struct {
		name  string
		match string
		want  string
	}{
		{"Z_PRIMARYKEY", "Z_PRIMARYKEY", "resolve entities"},
		{"ZACCOUNT", "ZTYPENAME", "read accounts"},
		{"ZRECONCILERECORD", "ZRECONCILERECORD", "read reconcile records"},
		{"ZTAG categories", "ZPARENTCATEGORY", "read categories"},
		{"ZTAG tags", "COALESCE(ZNAME, '')\nFROM ZTAG", "read tags"},
		{"ZUSERPAYEE", "ZUSERPAYEE", "read payees"},
		{"ZTRANSACTION", "ZPOSTEDDATE", "read transactions"},
		{"ZCASHFLOWTRANSACTIONENTRY", "FROM ZCASHFLOWTRANSACTIONENTRY", "read splits"},
		{"Z_15USERTAGS", "Z_15USERTAGS", "read split tags"},
		{"ZBUDGETLINEITEM", "FROM ZBUDGETLINEITEM", "read budget line items"},
		{"ZLOANSPLITENTRY", "FROM ZLOANSPLITENTRY", "read loan split entries"},
		{"ZACCOUNT loan interest", "ZLOANINTERESTCATEGORY", "read loan interest categories"},
		{"ZQUICKFILLRULESPLITENTRY", "FROM ZQUICKFILLRULESPLITENTRY", "read quickfill rule split entries"},
		{"ZPRODUCTSERVICE", "FROM ZPRODUCTSERVICE", "read product and service categories"},
		{"ZCUSTOMERCREDITLINEITEM", "FROM ZCUSTOMERCREDITLINEITEM", "read customer credit line item categories"},
		{"ZSECURITY", "ZTICKER", "read securities"},
		{"ZSECURITYQUOTE", "ZCLOSINGPRICE", "read prices"},
		{"ZPOSITION", "FROM ZPOSITION", "read positions"},
		{"ZLOT", "FROM ZLOT", "read lots"},
		{"ZTRANSACTION investments", "ZUNITS", "read investment transactions"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := importer.NewServer(importer.WithStore(&fakeStore{}), importer.WithSourceOpener(openerScanFailingOn(c.match, errBoom)))

			_, err := srv.Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			require.ErrorIs(t, err, errBoom)
			require.ErrorContains(t, err, c.want)
		})
	}
}

func Test_import_propagates_a_store_replace_error(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}
	errBoom := errStoreDiskFull
	fake.failNext(errBoom)

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, errBoom)
}

func Test_UnmappableError_matches_ErrUnmappable_and_keeps_its_reason(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("import: %w", &importer.UnmappableError{Reason: "boom"})

	require.ErrorIs(t, err, store.ErrUnmappable)
	assert.Equal(t, "boom", errors.Unwrap(err).Error())
}

func Test_UnmappableError_Error_returns_the_reason(t *testing.T) {
	t.Parallel()
	err := &importer.UnmappableError{Reason: "boom"}

	assert.Equal(t, "boom", err.Error())
}
