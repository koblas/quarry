package main

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// "Auto" is the control: its key differs. The income "Grocery" shares the
// expense key but is a different kind, so it joins no group.
func Test_run_sync_records_similar_expense_categories_and_not_the_income_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	groceryPK := b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(2))})
	b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{"similar-categories:grocery": "similar-categories"}, stringMap(t, db,
		"SELECT id, type FROM findings WHERE type = 'similar-categories'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("cat-%d", groceriesPK): "NULL|NULL",
		fmt.Sprintf("cat-%d", groceryPK):   "NULL|NULL",
	}, stringMap(t, db, `SELECT fi.category_id, COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.payee_id, 'NULL')
		FROM finding_items fi
		WHERE fi.finding_id = 'similar-categories:grocery'`))
}
