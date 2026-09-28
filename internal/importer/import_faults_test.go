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

func openerFailingOn(match string, err error) importer.SourceOpener {
	return func(ctx context.Context, path string) (importer.Source, error) {
		real, openErr := sqlite.OpenReadOnly(ctx, path)
		if openErr != nil {
			return nil, openErr
		}
		return &faultingSource{real: real, match: match, err: err}, nil
	}
}

// Import must propagate a failure from every source query it issues, not
// swallow it — one row per table the importer reads from.
func Test_import_propagates_a_fault_from_every_source_query(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: v9fixture.Int64Ptr(1)})
	b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: catPK})
	b.LinkUserTag(entryPK, tagPK)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	errBoom := errors.New("boom")
	cases := []struct {
		name  string
		match string
	}{
		{"Z_PRIMARYKEY", "Z_PRIMARYKEY"},
		{"ZACCOUNT", "ZTYPENAME"},
		{"ZRECONCILERECORD", "ZRECONCILERECORD"},
		{"ZTAG categories", "ZPARENTCATEGORY"},
		{"ZTAG tags", "COALESCE(ZNAME, '')\nFROM ZTAG"},
		{"ZUSERPAYEE", "ZUSERPAYEE"},
		{"ZTRANSACTION ids", "ZDELETIONCOUNT, 0) FROM ZTRANSACTION"},
		{"ZTRANSACTION", "ZPOSTEDDATE"},
		{"ZCASHFLOWTRANSACTIONENTRY", "FROM ZCASHFLOWTRANSACTIONENTRY"},
		{"Z_15USERTAGS", "Z_15USERTAGS"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := importer.NewServer(importer.WithStore(&fakeStore{}), importer.WithSourceOpener(openerFailingOn(c.match, errBoom)))

			_, err := srv.Import(t.Context(), bundle.DataPath)

			require.ErrorIs(t, err, errBoom)
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
		return f.real.QueryRows(ctx, query, args, func(scan func(dest ...any) error) error {
			return row(func(dest ...any) error { return f.err })
		})
	}
	return f.real.QueryRows(ctx, query, args, row)
}

func (f *scanFaultingSource) Close() error { return f.real.Close() }

func openerScanFailingOn(match string, err error) importer.SourceOpener {
	return func(ctx context.Context, path string) (importer.Source, error) {
		real, openErr := sqlite.OpenReadOnly(ctx, path)
		if openErr != nil {
			return nil, openErr
		}
		return &scanFaultingSource{real: real, match: match, err: err}, nil
	}
}

// Import must propagate a Scan failure on a row it already fetched, not
// just a failure to run the query itself — one row per table.
func Test_import_propagates_a_scan_fault_from_every_source_query(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: v9fixture.Int64Ptr(1)})
	b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: catPK})
	b.LinkUserTag(entryPK, tagPK)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	errBoom := errors.New("boom")
	cases := []struct {
		name  string
		match string
	}{
		{"Z_PRIMARYKEY", "Z_PRIMARYKEY"},
		{"ZACCOUNT", "ZTYPENAME"},
		{"ZRECONCILERECORD", "ZRECONCILERECORD"},
		{"ZTAG categories", "ZPARENTCATEGORY"},
		{"ZTAG tags", "COALESCE(ZNAME, '')\nFROM ZTAG"},
		{"ZUSERPAYEE", "ZUSERPAYEE"},
		{"ZTRANSACTION ids", "ZDELETIONCOUNT, 0) FROM ZTRANSACTION"},
		{"ZTRANSACTION", "ZPOSTEDDATE"},
		{"ZCASHFLOWTRANSACTIONENTRY", "FROM ZCASHFLOWTRANSACTIONENTRY"},
		{"Z_15USERTAGS", "Z_15USERTAGS"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := importer.NewServer(importer.WithStore(&fakeStore{}), importer.WithSourceOpener(openerScanFailingOn(c.match, errBoom)))

			_, err := srv.Import(t.Context(), bundle.DataPath)

			require.ErrorIs(t, err, errBoom)
		})
	}
}

func Test_import_propagates_a_store_replace_error(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}
	errBoom := errors.New("disk full")
	fake.failNext(errBoom)

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, errBoom)
}
