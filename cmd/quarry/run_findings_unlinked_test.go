// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unlinkedPairs is the txn primary keys of two unlinked pairs: -500.00 in Chequing categorized Bills with +500.00 in
// Visa split between Bills and Household, and on later dates -75.00 and +75.00 with no category and no payee.
type unlinkedPairs struct {
	outBig, inBig, outSmall, inSmall int64
}

// syncUnlinkedPairs syncs a Quicken file holding the two pairs into the store under home.
func syncUnlinkedPairs(t *testing.T, home string) unlinkedPairs {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	visaPaymentPK := b.Payee(v9fixture.PayeeRow{Name: "Visa payment"})
	thankYouPK := b.Payee(v9fixture.PayeeRow{Name: "Payment thank you"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	var p unlinkedPairs
	p.outBig = categorizedPayeeTxn(b, chequingPK, visaPaymentPK, billsPK, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "-500.00")
	bigDay := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	p.inBig = b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "500.00", PostedDate: &bigDay, Payee: thankYouPK})
	b.Entry(v9fixture.EntryRow{Parent: p.inBig, Amount: "300.00", CategoryTag: billsPK})
	b.Entry(v9fixture.EntryRow{Parent: p.inBig, Amount: "200.00", CategoryTag: householdPK})
	outDay, inDay := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	p.outSmall = b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-75.00", PostedDate: &outDay})
	b.Entry(v9fixture.EntryRow{Parent: p.outSmall, Amount: "-75.00"})
	p.inSmall = b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "75.00", PostedDate: &inDay})
	b.Entry(v9fixture.EntryRow{Parent: p.inSmall, Amount: "75.00"})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return p
}

func Test_run_findings_lists_an_unlinked_transfer_pair_with_its_category_cells(t *testing.T) {
	home := newHome(t)
	p := syncUnlinkedPairs(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "unlinked-transfer"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Unlinked transfers (2): make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts
  unlinked-transfer:txn-%[3]d+txn-%[4]d
    2026-07-10  Chequing (CAD)  (no payee)  -75.00  (uncategorized)
    2026-07-11  Visa (CAD)      (no payee)   75.00  (uncategorized)
  unlinked-transfer:txn-%[1]d+txn-%[2]d
    2026-07-02  Chequing (CAD)  Visa payment       -500.00  Bills
    2026-07-03  Visa (CAD)      Payment thank you   500.00  (split)

2 open findings
Ignore a finding by adding its id to findings.ignore in %[5]s; see quarry findings --help
`, p.outBig, p.inBig, p.outSmall, p.inSmall, configShown), stdout.String())
}

func Test_run_findings_json_gives_an_unlinked_transfer_item_its_category_path_or_null(t *testing.T) {
	home := newHome(t)
	p := syncUnlinkedPairs(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "unlinked-transfer"})

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			ID    string `json:"id"`
			Items []struct {
				TransactionID string  `json:"transaction_id"`
				Category      *string `json:"category"`
				CategoryID    *string `json:"category_id"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 2)
	got := map[string]*string{}
	for _, f := range doc.Findings {
		for _, item := range f.Items {
			got[item.TransactionID] = item.Category
			assert.Nil(t, item.CategoryID)
		}
	}
	assert.Equal(t, map[string]*string{
		fmt.Sprintf("txn-%d", p.outBig): new("Bills"), fmt.Sprintf("txn-%d", p.inBig): nil,
		fmt.Sprintf("txn-%d", p.outSmall): nil, fmt.Sprintf("txn-%d", p.inSmall): nil,
	}, got)
}
