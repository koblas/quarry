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

// Shell is the control: it moves from Auto to Auto:Fuel once and stays there.
func Test_run_sync_records_costco_as_mixed_categories_and_not_shell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	costcoPK := b.Payee(v9fixture.PayeeRow{Name: "Costco"})
	shellPK := b.Payee(v9fixture.PayeeRow{Name: "Shell"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	autoPK := b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})
	fuelPK := b.Category(v9fixture.TagRow{Name: "Fuel", Type: new(int64(1)), ParentCategory: autoPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend := func(payee, category int64, amount string) {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	}
	spend(costcoPK, groceriesPK, "-101.00")
	spend(costcoPK, householdPK, "-102.00")
	spend(costcoPK, groceriesPK, "-103.00")
	spend(costcoPK, fuelPK, "-104.00")
	spend(shellPK, autoPK, "-51.00")
	spend(shellPK, autoPK, "-52.00")
	spend(shellPK, fuelPK, "-53.00")
	spend(shellPK, fuelPK, "-54.00")

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("mixed-categories:payee-%d", costcoPK)
	assert.Equal(t, map[string]string{id: "mixed-categories"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'mixed-categories'"))
	assert.Equal(t, map[string]string{
		"Groceries": "Costco|NULL", "Household": "Costco|NULL", "Fuel": "Costco|NULL",
	}, stringMap(t, db, `SELECT c.name, p.name || '|' || COALESCE(fi.transaction_id, 'NULL')
		FROM finding_items fi
		JOIN categories c ON c.id = fi.category_id
		JOIN payees p ON p.id = fi.payee_id
		WHERE fi.finding_id LIKE 'mixed-categories:%'`))
}
