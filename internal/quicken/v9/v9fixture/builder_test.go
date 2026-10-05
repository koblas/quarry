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

// Proves Builder seeds every row kind WriteBundle exposes and reads back
// through the same platform/sqlite surface the importer will use: entity
// numbers land on ZTRANSACTION/ZTAG/Z_PRIMARYKEY, money round trips through
// SQLite's NUMERIC affinity, and Z_15USERTAGS survives with literal columns.
func Test_builder_seeds_every_row_kind_and_reads_back(t *testing.T) {
	postedDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	b := v9fixture.NewBuilder().WithEntity("UserTag", 9001)

	accountPK := b.Account(v9fixture.AccountRow{
		Name: "Checking", Type: "CHECKING", Currency: "CAD", Active: true,
	})
	payeePK := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	categoryPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
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
	b.Reconcile(v9fixture.ReconcileRow{Account: accountPK, EndDate: &postedDate, EndingBalance: "100.00"})

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
	// SQLite's NUMERIC affinity collapses a whole-valued decimal string to INTEGER, same as ZAMOUNT below.
	assert.Equal(t, "100", queryString(t, db, "SELECT ZENDINGBALANCE FROM ZRECONCILERECORD WHERE ZACCOUNT = ?", accountPK))

	linked := queryInt(t, db, "SELECT count(*) FROM Z_15USERTAGS WHERE Z_15CASHFLOWTRANSACTIONENTRIES = ? AND Z_76USERTAGS = ?", entryPK, tagPK)
	assert.Equal(t, int64(1), linked)

	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZAMOUNT) FROM ZTRANSACTION WHERE Z_PK = ?", txnPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZAMOUNT) FROM ZTRANSACTION WHERE Z_PK = ?", wholeTxnPK))
	assert.Equal(t, "12.345", queryString(t, db, "SELECT ZAMOUNT FROM ZTRANSACTION WHERE Z_PK = ?", preciseTxnPK))

	assert.Equal(t, tagPK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", 9001))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", v9fixture.EntCategoryTag))
}

// Deleted rows and a set (non-nil) status are both zero-value-shaped in Go
// (false, nil), so the acceptance test above — which never sets either —
// cannot prove Builder writes them; this does, reading each back.
func Test_builder_seeds_a_deleted_row_and_a_transaction_status(t *testing.T) {
	b := v9fixture.NewBuilder()
	accountPK := b.Account(v9fixture.AccountRow{Name: "Closed", Deleted: true})
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: accountPK, Amount: "1.00", Status: &reconciled})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, int64(1), queryInt(t, db, "SELECT ZDELETIONCOUNT FROM ZACCOUNT WHERE Z_PK = ?", accountPK))
	assert.Equal(t, int64(2), queryInt(t, db, "SELECT ZRECONCILESTATUS FROM ZTRANSACTION WHERE Z_PK = ?", txnPK))
}

func Test_builder_seeds_the_category_references_the_unused_category_check_reads(t *testing.T) {
	b := v9fixture.NewBuilder()
	categoryPK := b.Category(v9fixture.TagRow{Name: "Charity", Type: new(int64(1))})
	accountPK := b.Account(v9fixture.AccountRow{Name: "Mortgage", LoanInterestCategory: categoryPK})
	noInterestPK := b.Account(v9fixture.AccountRow{Name: "Chequing"})
	budgetPK := b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: categoryPK})
	deletedBudgetPK := b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: categoryPK, Deleted: true})
	loanPK := b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: categoryPK})
	deletedLoanPK := b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: categoryPK, Deleted: true})
	quickfillPK := b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: categoryPK})
	deletedQuickfillPK := b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: categoryPK, Deleted: true})
	productPK := b.ProductService(v9fixture.ProductServiceRow{Category: categoryPK})
	deletedProductPK := b.ProductService(v9fixture.ProductServiceRow{Category: categoryPK, Deleted: true})
	creditPK := b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: categoryPK})
	deletedCreditPK := b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: categoryPK, Deleted: true})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZLOANINTERESTCATEGORY FROM ZACCOUNT WHERE Z_PK = ?", accountPK))
	assert.Equal(t, int64(0), queryInt(t, db, "SELECT count(*) FROM ZACCOUNT WHERE Z_PK = ? AND ZLOANINTERESTCATEGORY IS NOT NULL", noInterestPK))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZCATEGORYTAG FROM ZBUDGETLINEITEM WHERE Z_PK = ? AND ZDELETIONCOUNT = 0", budgetPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT ZDELETIONCOUNT FROM ZBUDGETLINEITEM WHERE Z_PK = ?", deletedBudgetPK))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZCATEGORY FROM ZLOANSPLITENTRY WHERE Z_PK = ? AND ZDELETIONCOUNT = 0", loanPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT count(*) FROM ZLOANSPLITENTRY WHERE Z_PK = ? AND ZDELETIONCOUNT = 1 AND ZCATEGORY = ?", deletedLoanPK, categoryPK))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZCATEGORYTAG FROM ZQUICKFILLRULESPLITENTRY WHERE Z_PK = ? AND ZDELETIONCOUNT = 0", quickfillPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT count(*) FROM ZQUICKFILLRULESPLITENTRY WHERE Z_PK = ? AND ZDELETIONCOUNT = 1 AND ZCATEGORYTAG = ?", deletedQuickfillPK, categoryPK))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZCATEGORY FROM ZPRODUCTSERVICE WHERE Z_PK = ? AND ZDELETIONCOUNT = 0", productPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT count(*) FROM ZPRODUCTSERVICE WHERE Z_PK = ? AND ZDELETIONCOUNT = 1 AND ZCATEGORY = ?", deletedProductPK, categoryPK))
	assert.Equal(t, categoryPK, queryInt(t, db, "SELECT ZCATEGORY FROM ZCUSTOMERCREDITLINEITEM WHERE Z_PK = ? AND ZDELETIONCOUNT = 0", creditPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT count(*) FROM ZCUSTOMERCREDITLINEITEM WHERE Z_PK = ? AND ZDELETIONCOUNT = 1 AND ZCATEGORY = ?", deletedCreditPK, categoryPK))
}

func Test_builder_seeds_securities_and_their_quotes_with_entity_rows(t *testing.T) {
	quoteDay := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b := v9fixture.NewBuilder().WithEntity("SecurityQuote", 9068)
	securityPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	bareSecurityPK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Deleted: true})
	quotePK := b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: securityPK, QuoteDate: &quoteDay, ClosingPrice: "12.5"})
	wholeQuotePK := b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: securityPK, QuoteDate: &quoteDay, ClosingPrice: "3"})
	bareQuotePK := b.SecurityQuote(v9fixture.SecurityQuoteRow{Deleted: true})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, "Acme Corp", queryString(t, db, "SELECT ZNAME FROM ZSECURITY WHERE Z_PK = ?", securityPK))
	assert.Equal(t, "ACME", queryString(t, db, "SELECT ZTICKER FROM ZSECURITY WHERE Z_PK = ?", securityPK))
	assert.Equal(t, "CAD", queryString(t, db, "SELECT ZCURRENCY FROM ZSECURITY WHERE Z_PK = ?", securityPK))
	assert.Equal(t, int64(v9fixture.EntSecurity), queryInt(t, db, "SELECT Z_ENT FROM ZSECURITY WHERE Z_PK = ?", securityPK))
	assert.Equal(t, int64(1), queryInt(t, db, "SELECT count(*) FROM ZSECURITY WHERE Z_PK = ? AND ZTICKER IS NULL AND ZCURRENCY IS NULL AND ZDELETIONCOUNT = 1", bareSecurityPK))
	assert.Equal(t, securityPK, queryInt(t, db, "SELECT ZSECURITY FROM ZSECURITYQUOTE WHERE Z_PK = ?", quotePK))
	assert.Equal(t, int64(9068), queryInt(t, db, "SELECT Z_ENT FROM ZSECURITYQUOTE WHERE Z_PK = ?", quotePK))
	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZCLOSINGPRICE) FROM ZSECURITYQUOTE WHERE Z_PK = ?", quotePK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZCLOSINGPRICE) FROM ZSECURITYQUOTE WHERE Z_PK = ?", wholeQuotePK))
	assert.Equal(t, int64(1), queryInt(t, db,
		"SELECT count(*) FROM ZSECURITYQUOTE WHERE Z_PK = ? AND ZSECURITY IS NULL AND ZQUOTEDATE IS NULL"+
			" AND ZCLOSINGPRICE IS NULL AND ZDELETIONCOUNT = 1", bareQuotePK))
	assert.Equal(t, bareSecurityPK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", v9fixture.EntSecurity))
	assert.Equal(t, bareQuotePK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", 9068))
}

func Test_builder_seeds_positions_and_investment_transaction_fields(t *testing.T) {
	buyType := int64(3)
	b := v9fixture.NewBuilder().WithEntity("Position", 9049)
	accountPK := b.Account(v9fixture.AccountRow{Name: "Brokerage"})
	securityPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp"})
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: securityPK})
	bareDeletedPositionPK := b.Position(v9fixture.PositionRow{Deleted: true})
	fullPK := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: &buyType, Position: positionPK,
		Units: "2.5", Numerator: "1", Denominator: "12", Commission: "1.50", CostBasis: "1000.50",
	})
	wholePK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Units: "10", CostBasis: "1000"})
	barePK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, int64(9049), queryInt(t, db, "SELECT Z_ENT FROM ZPOSITION WHERE Z_PK = ?", positionPK))
	assert.Equal(t, securityPK, queryInt(t, db, "SELECT ZSECURITY FROM ZPOSITION WHERE Z_PK = ?", positionPK))
	assert.Equal(t, accountPK, queryInt(t, db, "SELECT ZACCOUNT FROM ZPOSITION WHERE Z_PK = ?", positionPK))
	assert.Equal(t, int64(1), queryInt(t, db,
		"SELECT count(*) FROM ZPOSITION WHERE Z_PK = ? AND ZACCOUNT IS NULL AND ZSECURITY IS NULL AND ZDELETIONCOUNT = 1", bareDeletedPositionPK))
	assert.Equal(t, bareDeletedPositionPK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", 9049))
	assert.Equal(t, int64(v9fixture.EntInvestmentTransaction), queryInt(t, db, "SELECT Z_ENT FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, buyType, queryInt(t, db, "SELECT ZTYPE FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, positionPK, queryInt(t, db, "SELECT ZPOSITION FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZUNITS) FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZUNITS) FROM ZTRANSACTION WHERE Z_PK = ?", wholePK))
	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZCOMMISSION) FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZCOSTBASIS) FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZCOSTBASIS) FROM ZTRANSACTION WHERE Z_PK = ?", wholePK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZNUMERATOR) FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZDENOMINATOR) FROM ZTRANSACTION WHERE Z_PK = ?", fullPK))
	assert.Equal(t, int64(1), queryInt(t, db,
		"SELECT count(*) FROM ZTRANSACTION WHERE Z_PK = ? AND ZTYPE IS NULL AND ZPOSITION IS NULL AND ZUNITS IS NULL"+
			" AND ZNUMERATOR IS NULL AND ZDENOMINATOR IS NULL AND ZCOMMISSION IS NULL AND ZCOSTBASIS IS NULL", barePK))
}

func Test_builder_seeds_lots(t *testing.T) {
	b := v9fixture.NewBuilder().WithEntity("Lot", 9044)
	accountPK := b.Account(v9fixture.AccountRow{Name: "Brokerage"})
	securityPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp"})
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: securityPK})
	fullPK := b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "2.5"})
	wholePK := b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "10"})
	bareDeletedPK := b.Lot(v9fixture.LotRow{Deleted: true})

	bundle := b.WriteBundle(t, t.TempDir())

	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, int64(9044), queryInt(t, db, "SELECT Z_ENT FROM ZLOT WHERE Z_PK = ?", fullPK))
	assert.Equal(t, positionPK, queryInt(t, db, "SELECT ZPOSITION FROM ZLOT WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "real", queryString(t, db, "SELECT typeof(ZLATESTUNITS) FROM ZLOT WHERE Z_PK = ?", fullPK))
	assert.Equal(t, "integer", queryString(t, db, "SELECT typeof(ZLATESTUNITS) FROM ZLOT WHERE Z_PK = ?", wholePK))
	assert.Equal(t, int64(1), queryInt(t, db,
		"SELECT count(*) FROM ZLOT WHERE Z_PK = ? AND ZPOSITION IS NULL AND ZLATESTUNITS IS NULL AND ZDELETIONCOUNT = 1", bareDeletedPK))
	assert.Equal(t, bareDeletedPK, queryInt(t, db, "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_ENT = ?", 9044))
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
