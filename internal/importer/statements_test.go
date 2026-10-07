package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// The account's reconciled sum is 100.00, so only the 100.00 statement matches; each case also adds one it must not pick.
func Test_import_checks_the_balance_against_the_newest_live_statement(t *testing.T) {
	t.Parallel()
	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		statements func(b *v9fixture.Builder, acctPK int64)
	}{
		{name: "uses_the_newest_statement_by_date_though_it_was_recorded_first", statements: func(b *v9fixture.Builder, acctPK int64) {
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "999.00"})
		}},
		{name: "breaks_a_date_tie_with_the_higher_source_id", statements: func(b *v9fixture.Builder, acctPK int64) {
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "999.00"})
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
		}},
		{name: "ignores_a_deleted_newer_statement", statements: func(b *v9fixture.Builder, acctPK int64) {
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &mar, EndingBalance: "1.00", Deleted: true})
		}},
		{name: "ignores_an_older_statements_bad_balance", statements: func(b *v9fixture.Builder, acctPK int64) {
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "12.345"})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := chequingWithOneReconciledTxn(b, "100.00")
			c.statements(b, acctPK)

			_, result := importOK(t, b)

			assert.Empty(t, result.Validation.Balances.Mismatched)
		})
	}
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

// The record's balance is unreadable, so the import would refuse it if it were checked.
func Test_import_skips_a_reconcile_record_on_an_account_it_does_not_import(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		account func(b *v9fixture.Builder) int64
	}{
		{name: "deleted_account", account: func(b *v9fixture.Builder) int64 {
			return b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
		}},
		{name: "missing_account", account: func(*v9fixture.Builder) int64 { return 999 }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
			b.Reconcile(v9fixture.ReconcileRow{Account: c.account(b), EndDate: &feb, EndingBalance: "not-a-number"})

			importOK(t, b)
		})
	}
}

func Test_import_refuses_a_dated_statement_balance_quarry_cannot_read(t *testing.T) {
	t.Parallel()
	const prefix = `the 2026-02-28 statement for "Chequing" `
	cases := []struct {
		name    string
		balance string
		want    string
	}{
		{name: "no_balance", balance: "", want: prefix + "has no balance"},
		{name: "more_than_2_decimal_places", balance: "12.345", want: prefix + "has a balance of 12.345, which has more than 2 decimal places"},
		{name: "too_large_for_quarry", balance: "10000000000000.5", want: prefix + "has a balance of 10000000000000.5, which is too large for quarry's amounts"},
		{name: "text_balance", balance: "not-a-number", want: prefix + "has a balance that is not a number"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newChequing(b)
			feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: c.balance})

			reason, _ := importRefused(t, b)

			assert.Equal(t, c.want, reason)
		})
	}
}

func Test_import_refuses_an_undated_statement(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "100.00"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) has no date`, reason)
}

func Test_import_refuses_an_undated_statement_with_a_bad_balance_using_the_source_id_subject(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		balance string
		want    string
	}{
		{name: "more_than_2_decimal_places", balance: "12.345", want: "has a balance of 12.345, which has more than 2 decimal places"},
		{name: "too_large_for_quarry", balance: "10000000000000.5", want: "has a balance of 10000000000000.5, which is too large for quarry's amounts"},
		{name: "text_balance", balance: "not-a-number", want: "has a balance that is not a number"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newChequing(b)
			pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: c.balance})

			reason, _ := importRefused(t, b)

			assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) `+c.want, reason)
		})
	}
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
