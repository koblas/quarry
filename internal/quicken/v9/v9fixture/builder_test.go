package v9fixture_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Proves Builder seeds every row kind WriteBundle exposes and that they
// read back through the same platform/sqlite surface 01a's importer will
// use: overridden entity numbers land on ZTRANSACTION/ZTAG, money round
// trips through SQLite's own NUMERIC affinity (whole values as INTEGER,
// fractional as REAL, and a third decimal place is still storable), and the
// Z_15USERTAGS link survives with its literal (non-entity-derived) columns.
func Test_builder_seeds_every_row_kind_and_reads_back(t *testing.T) {
	postedDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	b := v9fixture.NewBuilder().WithEntity("UserTag", 9001)

	accountPK := b.Account(v9fixture.AccountRow{
		Name: "Checking", Type: "CHECKING", Currency: "CAD", Active: true,
	})
	payeePK := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	categoryPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: 1})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	txnPK := b.Transaction(v9fixture.TransactionRow{
		Account: accountPK, Amount: "12.34", PostedDate: &postedDate, Payee: payeePK,
	})
	wholeTxnPK := b.Transaction(v9fixture.TransactionRow{
		Account: accountPK, Amount: "12.00", PostedDate: &postedDate, Payee: payeePK,
	})
	preciseTxnPK := b.Transaction(v9fixture.TransactionRow{
		Account: accountPK, Amount: "12.345", PostedDate: &postedDate, Payee: payeePK,
	})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "12.34", CategoryTag: categoryPK})
	b.LinkUserTag(entryPK, tagPK)
	b.Reconcile(v9fixture.ReconcileRow{Account: accountPK, EndDate: postedDate, EndingBalance: "100.00"})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, "Checking", queryString(t, db, "SELECT ZNAME FROM ZACCOUNT WHERE Z_PK = ?", accountPK))
	assert.Equal(t, "Coffee Shop", queryString(t, db, "SELECT ZNAME FROM ZUSERPAYEE WHERE Z_PK = ?", payeePK))
	assert.Equal(t, "Groceries", queryString(t, db, "SELECT ZNAME FROM ZTAG WHERE Z_PK = ?", categoryPK))
	assert.Equal(t, int64(v9fixture.EntCategoryTag), queryInt(t, db, "SELECT Z_ENT FROM ZTAG WHERE Z_PK = ?", categoryPK))
	assert.Equal(t, "Reimbursable", queryString(t, db, "SELECT ZNAME FROM ZTAG WHERE Z_PK = ?", tagPK))
	assert.Equal(t, int64(9001), queryInt(t, db, "SELECT Z_ENT FROM ZTAG WHERE Z_PK = ?", tagPK))
	assert.Equal(t, int64(v9fixture.EntCashFlowTransaction), queryInt(t, db, "SELECT Z_ENT FROM ZTRANSACTION WHERE Z_PK = ?", txnPK))
	// SQLite's own NUMERIC affinity collapses a whole-valued decimal string
	// to INTEGER on write, same as ZAMOUNT below; "100" is what a real v9
	// file returns for an even-dollar balance.
	assert.Equal(t, "100", queryString(t, db, "SELECT ZENDINGBALANCE FROM ZRECONCILERECORD WHERE ZACCOUNT = ?", accountPK))

	linked := queryInt(t, db, "SELECT count(*) FROM Z_15USERTAGS WHERE Z_15CASHFLOWTRANSACTIONENTRIES = ? AND Z_76USERTAGS = ?", entryPK, tagPK)
	assert.Equal(t, int64(1), linked)

	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZAMOUNT) FROM ZTRANSACTION WHERE Z_PK = ?", txnPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZAMOUNT) FROM ZTRANSACTION WHERE Z_PK = ?", wholeTxnPK))
	assert.Equal(t, "12.345", queryString(t, db, "SELECT ZAMOUNT FROM ZTRANSACTION WHERE Z_PK = ?", preciseTxnPK))
}

func queryString(t *testing.T, db *sqlite.DB, query string, args ...any) string {
	t.Helper()
	var got string
	require.NoError(t, db.QueryRows(t.Context(), query, args, func(scan func(dest ...any) error) error {
		return scan(&got)
	}))
	return got
}

func queryInt(t *testing.T, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()
	var got int64
	require.NoError(t, db.QueryRows(t.Context(), query, args, func(scan func(dest ...any) error) error {
		return scan(&got)
	}))
	return got
}
