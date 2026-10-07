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

// reconcileStatus is ZRECONCILESTATUS for a reconciled transaction.
const reconcileStatus = 2

// categorizedTxn adds a categorized transaction of amount dated day, so the only finding it can raise is a duplicate.
func categorizedTxn(b *v9fixture.Builder, account, category int64, day time.Time, amount string, status *int64) int64 {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day, Status: status})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	return txn
}

func Test_run_sync_records_two_same_amount_transactions_within_three_days_as_a_duplicate(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	first := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17", nil)
	second := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17", nil)

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second)
	assert.Equal(t, map[string]string{id: "duplicate"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'duplicate'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", first): id, fmt.Sprintf("txn-%d", second): id,
	}, stringMap(t, db, "SELECT transaction_id, finding_id FROM finding_items WHERE finding_id LIKE 'duplicate:%'"))
}

// The reconciled/uncleared pair is the control: one reconciled side is still flagged.
func Test_run_sync_does_not_flag_two_reconciled_look_alikes_as_a_duplicate(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	reconciled := new(int64(reconcileStatus))
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	categorizedTxn(b, chequingPK, foodPK, day, "-142.17", reconciled)
	categorizedTxn(b, chequingPK, foodPK, day.AddDate(0, 0, 1), "-142.17", reconciled)
	controlReconciled := categorizedTxn(b, chequingPK, foodPK, day, "-9.99", reconciled)
	controlUncleared := categorizedTxn(b, chequingPK, foodPK, day.AddDate(0, 0, 1), "-9.99", nil)

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{
		fmt.Sprintf("duplicate:txn-%d+txn-%d", controlReconciled, controlUncleared): "duplicate",
	}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'duplicate'"))
}
