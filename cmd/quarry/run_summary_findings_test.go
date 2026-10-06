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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncSummaryFindingsFixture syncs a file raising one duplicate pair, three uncategorized payees and one
// unclassified brokerage account under home, returning the duplicate's id and the brokerage account's.
func syncSummaryFindingsFixture(t *testing.T, home string) (string, string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	for i, payee := range []string{"Amazon", "Costco", "Shell"} {
		amount := fmt.Sprintf("-%d0.00", i+1)
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: b.Payee(v9fixture.PayeeRow{Name: payee})})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second), fmt.Sprintf("acct-%d", brokeragePK)
}

// summaryOf is quarry summary for January 2026, a month that has ended whatever the clock says.
func summaryOf(t *testing.T) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"summary", "--month", "2026-01"}, &stdout, &stderr), stderr.String())
	return stdout.String()
}

func Test_run_summary_findings_count_a_classified_investment_account_and_an_ignored_id(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	duplicate, brokerage := syncSummaryFindingsFixture(t, home)

	before := summaryOf(t)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n[accounts]\nnon-registered = [%q]\n", duplicate, brokerage))
	after := summaryOf(t)
	var status, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status"}, &status, &stderr), stderr.String())

	assert.Contains(t, before, "\nFindings  5 open")
	assert.Contains(t, after, "\nFindings  3 open, 1 ignored")
	assert.Contains(t, status.String(), "\nFindings  3 open, 1 ignored")
}
