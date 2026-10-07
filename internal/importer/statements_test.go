package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// The correct (later) record is inserted first, so a "last row scanned
// wins" bug would pick the wrong (earlier) one instead.
func Test_import_uses_the_newest_statement_by_date(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "999.00"})

	_, result := importOK(t, b)

	assert.Empty(t, result.Validation.Balances.Mismatched)
}

func Test_import_imports_a_statement_balance_carrying_float_residue_as_its_cent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.0000004"})

	_, result := importOK(t, b)

	assert.Equal(t, 1, result.Validation.Balances.Checked)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// The undated record is inserted first, so a "last row scanned wins" bug
// would pick the dated one instead and the import would succeed.
func Test_import_ranks_an_undated_statement_as_newest_over_a_dated_one(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "100.00"})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) has no date`, reason)
}

// Same ZENDDATE on two records: the higher Z_PK's balance wins.
func Test_import_breaks_a_statement_date_tie_with_the_higher_source_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "999.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})

	_, result := importOK(t, b)

	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// A deleted record dated after the true newest must not be selected: its
// balance would mismatch if it were.
func Test_import_ignores_a_deleted_newer_statement(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &mar, EndingBalance: "1.00", Deleted: true})

	_, result := importOK(t, b)

	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// The newest (good) record is inserted first, so a "last row scanned
// wins" bug would read the older, bad one and refuse the import instead.
func Test_import_ignores_an_older_statements_bad_balance(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "12.345"})

	_, result := importOK(t, b)

	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// A malformed reconcile record on an investment account must not refuse
// the import: investment accounts are counted, never checked.
func Test_import_skips_a_reconcile_record_on_an_investment_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	brokeragePK := newBrokerage(b)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: brokeragePK, EndDate: &feb, EndingBalance: "not-a-number"})

	_, result := importOK(t, b)

	assert.Equal(t, 1, result.Validation.Balances.InvestmentAccounts)
}

// A reconcile record on a deleted account must not refuse the import: the
// account it names was never mapped.
func Test_import_skips_a_reconcile_record_on_a_deleted_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	deletedPK := b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: deletedPK, EndDate: &feb, EndingBalance: "not-a-number"})

	importOK(t, b)
}

// A reconcile record whose ZACCOUNT points to no row at all must not
// refuse the import.
func Test_import_skips_a_reconcile_record_on_a_missing_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: 999, EndDate: &feb, EndingBalance: "not-a-number"})

	importOK(t, b)
}

func Test_import_refuses_a_statement_with_no_balance(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `the 2026-02-28 statement for "Chequing" has no balance`, reason)
}

func Test_import_refuses_an_undated_statement(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "100.00"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) has no date`, reason)
}

func Test_import_refuses_a_statement_with_more_than_2_decimal_places(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "12.345"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`the 2026-02-28 statement for "Chequing" has a balance of 12.345, which has more than 2 decimal places`,
		reason)
}

func Test_import_refuses_a_statement_with_a_balance_too_large_for_quarry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "10000000000000.5"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`the 2026-02-28 statement for "Chequing" has a balance of 10000000000000.5, which is too large for quarry's amounts`,
		reason)
}

func Test_import_refuses_a_statement_with_a_text_balance(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "not-a-number"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `the 2026-02-28 statement for "Chequing" has a balance that is not a number`, reason)
}

func Test_import_refuses_an_undated_statement_with_a_bad_balance_using_the_source_id_subject(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "12.345"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance of 12.345, which has more than 2 decimal places`,
		reason)
}

// Same as the precision case above, for the too-large fault.
func Test_import_refuses_an_undated_statement_with_a_too_large_balance_using_the_source_id_subject(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "10000000000000.5"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance of 10000000000000.5, which is too large for quarry's amounts`,
		reason)
}

// Same as the precision case above, for the not-a-number fault.
func Test_import_refuses_an_undated_statement_with_a_text_balance_using_the_source_id_subject(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "not-a-number"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance that is not a number`,
		reason)
}

// An account at Z_PK 0 exists, so a NULL account read as 0 would be checked against it.
func Test_import_skips_a_statement_with_no_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: 0, EndDate: &feb, EndingBalance: "999.00"})
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "INSERT INTO ZACCOUNT (Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZACTIVE) VALUES (0, 'Zero', 'CHECKING', 'CAD', 1)")

	_, result := importOKFrom(t, dataPath)

	assert.Equal(t, 1, result.Validation.Balances.Checked)
}
