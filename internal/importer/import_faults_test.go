package importer_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

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
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &posted, Position: positionPK, Units: "1"})
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
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: brokeragePK, Type: new(int64(3)), Amount: "1.00", PostedDate: &posted, Position: positionPK, Units: "1"})
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
