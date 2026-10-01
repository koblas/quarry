package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// "Brokerage Fees" is used only by an investment entry and "Charity" only by a
// budget line, neither of which the importer stores as a split.
func Test_run_sync_records_unused_categories_but_not_one_an_investment_or_budget_uses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	parkingPK := b.Category(v9fixture.TagRow{Name: "Parking", Type: new(int64(1))})
	feesPK := b.Category(v9fixture.TagRow{Name: "Brokerage Fees", Type: new(int64(1))})
	charityPK := b.Category(v9fixture.TagRow{Name: "Charity", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Old", Type: new(int64(1)), Hidden: true})
	vacationPK := b.Category(v9fixture.TagRow{Name: "Vacation", Type: new(int64(1))})
	hotelPK := b.Category(v9fixture.TagRow{Name: "Hotel", Type: new(int64(1)), ParentCategory: vacationPK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	buyPK := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyPK, Amount: "-400.00", CategoryTag: feesPK})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: charityPK})

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{
		fmt.Sprintf("unused-category:cat-%d", parkingPK):  "unused-category",
		fmt.Sprintf("unused-category:cat-%d", vacationPK): "unused-category",
	}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'unused-category'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("cat-%d", vacationPK): "NULL|NULL",
		fmt.Sprintf("cat-%d", hotelPK):    "NULL|NULL",
	}, stringMap(t, db, fmt.Sprintf(`SELECT fi.category_id, COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.payee_id, 'NULL')
		FROM finding_items fi
		WHERE fi.finding_id = 'unused-category:cat-%d'`, vacationPK)))
}
