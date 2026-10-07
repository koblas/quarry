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

// The CAD/USD pair is the control: same magnitude, opposite signs, within the window, but different currencies.
func Test_run_sync_records_opposite_amounts_in_two_cad_accounts_as_an_unlinked_transfer_and_not_the_cad_usd_pair(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	out := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "-500.00", nil)
	in := categorizedTxn(b, visaPK, foodPK, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), "500.00", nil)
	categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "-75.00", nil)
	categorizedTxn(b, savingsPK, foodPK, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "75.00", nil)

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("unlinked-transfer:txn-%d+txn-%d", out, in)
	assert.Equal(t, map[string]string{id: "unlinked-transfer"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'unlinked-transfer'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", out): id, fmt.Sprintf("txn-%d", in): id,
	}, stringMap(t, db, "SELECT transaction_id, finding_id FROM finding_items WHERE finding_id LIKE 'unlinked-transfer:%'"))
}
