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

func Test_run_accounts_lists_open_accounts_with_their_balances(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type        Currency       Balance  Status\n"+
		"Chequing     chequing    CAD          12,345.67\n"+
		"Old Savings  savings     CAD               0.00  inactive\n"+
		"RRSP         retirement  CAD       not imported\n"+
		"US Chequing  chequing    USD           8,310.00\n",
		stdout.String())
}

func Test_run_accounts_all_lists_closed_accounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account        Type         Currency       Balance  Status\n"+
		"Chequing       chequing     CAD          12,345.67\n"+
		"Old Savings    savings      CAD               0.00  inactive\n"+
		"RRSP           retirement   CAD       not imported\n"+
		"US Chequing    chequing     USD           8,310.00\n"+
		"Visa Infinite  credit_card  CAD          -1,204.17  closed\n",
		stdout.String())
}

func Test_run_accounts_refuses_when_home_is_unset(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry accounts again\n",
		stderr.String())
}

func Test_run_accounts_reports_exit_1_when_stdout_cannot_be_written(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var syncOut, syncErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncOut, &syncErr), syncErr.String())
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), errNoSpace.Error())
}

func Test_run_accounts_help_describes_the_command_without_needing_home(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts show "not imported": quarry does not
import investment transactions yet, so it cannot compute their balance.`)
	assert.Contains(t, stdout.String(), "      --all    include closed accounts\n")
}

// syncAccountsFixture builds the store from open CAD, USD and inactive accounts,
// a retirement account and a closed one; Chequing also holds a transaction a year ahead.
func syncAccountsFixture(t *testing.T, home string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	usChequing := b.Account(v9fixture.AccountRow{Name: "US Chequing", Type: "CHECKING", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Old Savings", Type: "SAVINGS", Currency: "CAD", Active: false})
	rrsp := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
	visa := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true})
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	now := time.Now()
	nextYear := time.Date(now.Year()+1, now.Month(), 1, 0, 0, 0, 0, time.UTC)
	addTransaction(b, chequing, "12400.00", past)
	addTransaction(b, chequing, "-54.33", past)
	addTransaction(b, chequing, "500.00", nextYear)
	addTransaction(b, usChequing, "8310.00", past)
	addTransaction(b, rrsp, "1000.00", past)
	addTransaction(b, visa, "-1204.17", past)
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	var syncOut, syncErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncOut, &syncErr), syncErr.String())
}

// addTransaction adds one transaction with a single split of the same amount.
func addTransaction(b *v9fixture.Builder, account int64, amount string, posted time.Time) {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
}

func syncClosedAccountsFixture(t *testing.T, home string, n int) {
	t.Helper()
	b := v9fixture.NewBuilder()
	for i := range n {
		b.Account(v9fixture.AccountRow{Name: fmt.Sprintf("Closed %d", i+1), Type: "CHECKING", Currency: "CAD", Closed: true, Active: true})
	}
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	var syncOut, syncErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncOut, &syncErr), syncErr.String())
}

func Test_run_accounts_says_how_to_list_them_when_every_account_is_closed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, "Account  Type  Currency  Balance  Status\n", stdout.String())
	assert.Equal(t, "quarry: all 3 accounts are closed; pass --all to list them\n", stderr.String())
}

func Test_run_accounts_all_lists_closed_accounts_without_a_note(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Closed 1  chequing  CAD")
}
