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
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinFirstFoundAt sets the build time and every finding's first_found_at to one fixed moment, so the findings stay new.
func pinFirstFoundAt(t *testing.T, home string) {
	t.Helper()
	editStore(t, home, "UPDATE findings SET first_found_at = TIMESTAMP '2026-09-30 14:15:02'; UPDATE store_info SET built_at = TIMESTAMP '2026-09-30 14:15:02'")
}

func Test_run_findings_json_prints_the_ruled_document_for_one_open_duplicate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	pinFirstFoundAt(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.JSONEq(t, fmt.Sprintf(`{
  "status": "open",
  "type": null,
  "counts": {"open": 1, "ignored": 0, "fixed": 0, "new": 1, "newly_fixed": 0},
  "findings": [
    {
      "id": "duplicate:txn-%[1]d+txn-%[2]d",
      "type": "duplicate",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Delete the extra one in Quicken, or ignore the pair if both are real",
      "items": [
        {"transaction_id": "txn-%[1]d", "split_id": null, "payee_id": null, "category_id": null,
         "date": "2026-08-03", "account_id": "acct-%[3]d", "account": "Chequing", "currency": "CAD",
         "payee": "Hydro One", "category": null, "amount": "-142.17",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null},
        {"transaction_id": "txn-%[2]d", "split_id": null, "payee_id": null, "category_id": null,
         "date": "2026-08-05", "account_id": "acct-%[3]d", "account": "Chequing", "currency": "CAD",
         "payee": "Hydro One", "category": null, "amount": "-142.17",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null}
      ]
    }
  ],
  "warnings": []
}`, first, second, chequingPK), stdout.String())
}

func Test_run_findings_json_items_of_a_one_sided_transfer_and_an_uncategorized_payee_carry_their_own_fields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CHECKING", Currency: "USD", Active: true})
	paymentPK := b.Payee(v9fixture.PayeeRow{Name: "Payment"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	transferDay := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-1200.50", PostedDate: &transferDay, Payee: paymentPK})
	transferLeg := b.Entry(v9fixture.EntryRow{Parent: transferTxn, Amount: "-1200.50", QuickenID: 3001, Transfer: "Savings"})
	uncategorizedDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	uncategorizedTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-10.00", PostedDate: &uncategorizedDay, Payee: amazonPK})
	uncategorizedSplit := b.Entry(v9fixture.EntryRow{Parent: uncategorizedTxn, Amount: "-10.00"})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	pinFirstFoundAt(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.JSONEq(t, fmt.Sprintf(`{
  "status": "open",
  "type": null,
  "counts": {"open": 2, "ignored": 0, "fixed": 0, "new": 2, "newly_fixed": 0},
  "findings": [
    {
      "id": "one-sided-transfer:xfer-%[1]d",
      "type": "one-sided-transfer",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
      "items": [
        {"transaction_id": "txn-%[2]d", "split_id": "split-%[1]d", "payee_id": null, "category_id": null,
         "date": "2024-02-01", "account_id": "acct-%[3]d", "account": "Visa", "currency": "USD",
         "payee": "Payment", "category": null, "amount": "-1200.50",
         "other_account": "Savings", "other_account_id": null, "transactions": null, "splits": null}
      ]
    },
    {
      "id": "uncategorized:payee-%[4]d",
      "type": "uncategorized",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Give this payee's splits a category in Quicken",
      "items": [
        {"transaction_id": "txn-%[5]d", "split_id": "split-%[6]d", "payee_id": null, "category_id": null,
         "date": "2026-03-01", "account_id": "acct-%[3]d", "account": "Visa", "currency": "USD",
         "payee": "Amazon", "category": null, "amount": "-10.00",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null}
      ]
    }
  ],
  "warnings": []
}`, transferLeg, transferTxn, visaPK, amazonPK, uncategorizedTxn, uncategorizedSplit), stdout.String())
}

func Test_run_findings_json_with_none_open_prints_empty_lists_not_null(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-1", 1)},
		spendSplit{id: "1", account: "acct-1", category: "cat-fuel", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 1), cents: -4500}))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.JSONEq(t, `{
  "status": "open",
  "type": null,
  "counts": {"open": 0, "ignored": 0, "fixed": 0, "new": 0, "newly_fixed": 0},
  "findings": [],
  "warnings": []
}`, stdout.String())
}

func Test_run_findings_json_lists_a_config_warning_without_the_prefix_and_prints_it_to_stderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-1", 1)},
		spendSplit{id: "1", account: "acct-1", category: "cat-fuel", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 1), cents: -4500}))
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{configShown + ": unknown key snapshot.keep; quarry ignores it"}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr.String())
}
