// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
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

// syncTimHortons syncs a Quicken file in which "TIM HORTONS #1234" has two transactions and "Tim Hortons" one,
// and returns their payee primary keys.
func syncTimHortons(t *testing.T, home string) (int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	numberedPK := b.Payee(v9fixture.PayeeRow{Name: "TIM HORTONS #1234"})
	plainPK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, payee := range []int64{numberedPK, numberedPK, plainPK} {
		day = day.AddDate(0, 0, 10)
		amount := fmt.Sprintf("-%d.01", i+3)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: groceriesPK})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return numberedPK, plainPK
}

func Test_run_findings_lists_payee_variants_with_a_row_per_payee(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncTimHortons(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--type", "payee-variants"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Payee variants (1 group): rename each group to one payee in Quicken and add a renaming rule
  payee-variants:tim-hortons  2 payees, 3 transactions
    TIM HORTONS #1234  2 transactions
    Tim Hortons         1 transaction

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown), stdout.String())
}

func Test_run_findings_json_gives_a_payee_variants_item_its_payee_and_count_and_null_transaction_fields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	numberedPK, plainPK := syncTimHortons(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json", "--type", "payee-variants"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			ID    string `json:"id"`
			Items []struct {
				Payee         string  `json:"payee"`
				PayeeID       string  `json:"payee_id"`
				Transactions  int     `json:"transactions"`
				Category      *string `json:"category"`
				TransactionID *string `json:"transaction_id"`
				Date          *string `json:"date"`
				Amount        *string `json:"amount"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 1)
	assert.Equal(t, "payee-variants:tim-hortons", doc.Findings[0].ID)
	items := doc.Findings[0].Items
	require.Len(t, items, 2)
	assert.Equal(t, []string{"TIM HORTONS #1234", fmt.Sprintf("payee-%d", numberedPK), "Tim Hortons", fmt.Sprintf("payee-%d", plainPK)},
		[]string{items[0].Payee, items[0].PayeeID, items[1].Payee, items[1].PayeeID})
	assert.Equal(t, []int{2, 1}, []int{items[0].Transactions, items[1].Transactions})
	assert.Equal(t, []*string{nil, nil, nil, nil}, []*string{items[0].Category, items[0].TransactionID, items[0].Date, items[0].Amount})
}
