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

func Test_run_accounts_lists_open_accounts_with_their_balances(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type        Currency     Balance  Status\n"+
		"Chequing     chequing    CAD        12,345.67\n"+
		"Old Savings  savings     CAD             0.00  inactive\n"+
		"RRSP         retirement  CAD       not valued\n"+
		"US Chequing  chequing    USD         8,310.00\n",
		stdout.String())
}

func Test_run_accounts_all_lists_closed_accounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account        Type         Currency     Balance  Status\n"+
		"Chequing       chequing     CAD        12,345.67\n"+
		"Old Savings    savings      CAD             0.00  inactive\n"+
		"RRSP           retirement   CAD       not valued\n"+
		"US Chequing    chequing     USD         8,310.00\n"+
		"Visa Infinite  credit_card  CAD        -1,204.17  closed\n",
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

func Test_run_accounts_reports_a_failed_stdout_write(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
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

Brokerage and retirement accounts show "not valued": quarry imports their
transactions and checks their share counts against Quicken, but does not
value holdings yet, so it cannot compute their balance.

A column shows each balance in the reporting currency (--currency, else
reporting.currency in the config file, else CAD) at today's Bank of
Canada rate, or the latest earlier one; --currency native leaves it
out. quarry does not add balances together: a total that leaves out
investment accounts would not be your net worth.`)
	assert.Regexp(t, `(?m)^ +--all +include closed accounts$`, stdout.String())
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

	syncBundle(t, bundle)
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

	syncBundle(t, bundle)
}

func Test_run_accounts_says_how_to_list_them_when_every_account_is_closed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, "Account  Type  Currency  Balance  Status\n", stdout.String())
	assert.Equal(t, "quarry: warning: all 3 accounts are closed; pass --all to list them\n", stderr.String())
}

func Test_run_accounts_all_lists_closed_accounts_without_a_note(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account   Type      Currency  Balance  Status\n"+
		"Closed 1  chequing  CAD          0.00  closed\n"+
		"Closed 2  chequing  CAD          0.00  closed\n"+
		"Closed 3  chequing  CAD          0.00  closed\n",
		stdout.String())
}

// syncNotInReportsFixture builds an open, an inactive and a closed account that
// Quicken leaves out of reports, plus an open one it includes.
func syncNotInReportsFixture(t *testing.T, home string) {
	t.Helper()
	off := new(int64(0))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Float", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: off})
	b.Account(v9fixture.AccountRow{Name: "Old Savings", Type: "SAVINGS", Currency: "CAD", Active: false, UsedInReports: off})
	b.Account(v9fixture.AccountRow{Name: "Old Visa", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true, UsedInReports: off})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

func Test_run_accounts_all_marks_accounts_left_out_of_reports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncNotInReportsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type         Currency  Balance  Status\n"+
		"Chequing     chequing     CAD          0.00\n"+
		"Float        chequing     CAD          0.00  not in reports\n"+
		"Old Savings  savings      CAD          0.00  inactive, not in reports\n"+
		"Old Visa     credit_card  CAD          0.00  closed, not in reports\n",
		stdout.String())
}

func syncLinkedTrackingFixture(t *testing.T, home string) {
	t.Helper()
	on, off := new(int64(1)), new(int64(0))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: off})
	b.Account(v9fixture.AccountRow{Name: "Linked", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: on})
	b.Account(v9fixture.AccountRow{Name: "Old Linked", Type: "SAVINGS", Currency: "CAD", Active: false, SimpleInvesting: on})
	b.Account(v9fixture.AccountRow{Name: "Gone Linked", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true, UsedInReports: off, SimpleInvesting: on})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

func Test_run_accounts_all_marks_accounts_that_use_linked_account_tracking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncLinkedTrackingFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all", "--currency", "native"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type         Currency  Balance  Status\n"+
		"Chequing     chequing     CAD          0.00\n"+
		"Gone Linked  credit_card  CAD          0.00  closed, not in reports, linked tracking\n"+
		"Linked       chequing     CAD          0.00  linked tracking\n"+
		"Old Linked   savings      CAD          0.00  inactive, linked tracking\n",
		stdout.String())
}
