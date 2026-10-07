// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// "Tim Hortons Cafe" is the control: its key (tim-hortons-cafe) differs.
func Test_run_sync_records_tim_hortons_variants_and_not_unrelated_payees(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	numberedPK := b.Payee(v9fixture.PayeeRow{Name: "TIM HORTONS #1234"})
	plainPK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons"})
	cafePK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons Cafe"})
	shellPK := b.Payee(v9fixture.PayeeRow{Name: "Shell"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend := func(payee int64, amount string) {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: groceriesPK})
	}
	spend(numberedPK, "-3.01")
	spend(numberedPK, "-3.02")
	spend(plainPK, "-4.01")
	spend(cafePK, "-5.01")
	spend(shellPK, "-6.01")

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{"payee-variants:tim-hortons": "payee-variants"}, stringMap(t, db,
		"SELECT id, type FROM findings WHERE type = 'payee-variants'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("payee-%d", numberedPK): "TIM HORTONS #1234|NULL|NULL",
		fmt.Sprintf("payee-%d", plainPK):    "Tim Hortons|NULL|NULL",
	}, stringMap(t, db, `SELECT fi.payee_id, p.name || '|' || COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.category_id, 'NULL')
		FROM finding_items fi
		JOIN payees p ON p.id = fi.payee_id
		WHERE fi.finding_id = 'payee-variants:tim-hortons'`))
}
