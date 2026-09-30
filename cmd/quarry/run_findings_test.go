// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_findings_lists_open_findings_by_type_with_their_fix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	paymentPK := b.Payee(v9fixture.PayeeRow{Name: "Payment"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	transferDay := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "1200.00", PostedDate: &transferDay, Payee: paymentPK})
	transferLeg := b.Entry(v9fixture.EntryRow{Parent: transferTxn, Amount: "1200.00", QuickenID: 3001, Transfer: "Savings"})
	for i, amount := range []string{"-10.00", "-20.00"} {
		uncategorizedDay := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &uncategorizedDay, Payee: amazonPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Possible duplicates (1): delete the extra one in Quicken, or ignore the pair if both are real
  duplicate:txn-%d+txn-%d
    2026-08-03  Chequing (CAD)  Hydro One           -142.17
    2026-08-05  Chequing (CAD)  HYDRO ONE NETWORKS  -142.17

One-sided transfers (1): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file
  one-sided-transfer:xfer-%d  2024-02-01  Visa (CAD)  Payment  1,200.00  other account: Savings (not in this file)

Uncategorized (1 payee, 2 splits): give each payee's splits a category in Quicken
  uncategorized:payee-%d  Amazon  2 splits  2026-03-01 to 2026-03-02

3 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, first, second, transferLeg, amazonPK, configShown), stdout.String())
}

func Test_run_findings_says_no_open_findings_when_the_store_has_none(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-1", 1)},
		spendSplit{id: "1", account: "acct-1", category: "cat-fuel", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 1), cents: -4500}))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "No open findings\n", stdout.String())
}

func Test_run_findings_refuses_a_store_built_before_findings_existed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStoreFixture(t, home, phaseOneImportRunsDDL+
		"CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);"+
		"INSERT INTO store_info VALUES (3, '0.3.0', TIMESTAMP '2026-09-27 14:30:05');")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: the store at "+abbreviated(t, storePathUnder(home), home)+
		" was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it\n",
		stderr.String())
}

// categorizedPayeeTxn adds a categorized transaction of amount by payee dated posted, so the only finding it can raise is a duplicate.
func categorizedPayeeTxn(b *v9fixture.Builder, account, payee, category int64, posted time.Time, amount string) int64 {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted, Payee: payee})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	return txn
}
