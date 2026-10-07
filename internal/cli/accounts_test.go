package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errStoreRead = errors.New("read store accounts: disk read failed")
	errNoSpace   = errors.New("write /dev/stdout: no space left on device")
)

var asOf = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

func closedAccounts(n int) store.AccountList {
	list := store.AccountList{AsOf: asOf}
	for range n {
		list.Accounts = append(list.Accounts, store.AccountBalance{ID: "acct", Name: "Old", Type: "chequing", Currency: "CAD", Closed: true, Active: true, Balance: big.NewInt(0), Cash: big.NewInt(0)})
	}
	return list
}

func executeAccounts(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr)
	return cli.Execute(t.Context(), append([]string{"accounts"}, args...), env)
}

func Test_accounts_all_closed_note(t *testing.T) {
	const header = "Account  Type  Currency  Balance  Status\n"
	open := store.AccountBalance{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true, Balance: big.NewInt(100), Cash: big.NewInt(100)}
	mixed := closedAccounts(2)
	mixed.Accounts = append(mixed.Accounts, open)
	cases := []struct {
		name     string
		list     store.AccountList
		args     []string
		human    string
		rows     int
		wantNote []string
	}{
		{name: "several closed accounts", list: closedAccounts(3), human: header, wantNote: []string{"all 3 accounts are closed; pass --all to list them"}},
		{name: "a thousands-grouped count", list: closedAccounts(1204), human: header, wantNote: []string{"all 1,204 accounts are closed; pass --all to list them"}},
		{name: "exactly one closed account", list: closedAccounts(1), human: header, wantNote: []string{"the only account is closed; pass --all to list it"}},
		{name: "no accounts in the store", list: store.AccountList{AsOf: asOf}, human: header, wantNote: []string{}},
		{
			name: "an open account among closed ones", list: mixed, rows: 1, wantNote: []string{},
			human: "Account   Type      Currency  Balance  Status\nChequing  chequing  CAD          1.00\n",
		},
		{
			name: "closed accounts asked for", list: closedAccounts(1), args: []string{"--all"}, rows: 1, wantNote: []string{},
			human: "Account  Type      Currency  Balance  Status\nOld      chequing  CAD          0.00  closed\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name+", human", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeAccounts(t, fakeReportStore{accounts: c.list}, &stdout, &stderr, append([]string{"--currency", "native"}, c.args...)...)

			require.NoError(t, err)
			assert.Equal(t, c.human, stdout.String())
			assert.Equal(t, stderrOf(c.wantNote), stderr.String())
		})
		t.Run(c.name+", json", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeAccounts(t, fakeReportStore{accounts: c.list}, &stdout, &stderr, append([]string{"--json", "--currency", "native"}, c.args...)...)

			require.NoError(t, err)
			var got struct {
				Accounts []json.RawMessage `json:"accounts"`
				Warnings []string          `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			assert.Len(t, got.Accounts, c.rows)
			assert.Equal(t, c.wantNote, got.Warnings)
			assert.Equal(t, stderrOf(c.wantNote), stderr.String())
		})
	}
}

func stderrOf(notes []string) string {
	var b bytes.Buffer
	for _, n := range notes {
		b.WriteString("quarry: warning: " + n + "\n")
	}
	return b.String()
}

func Test_accounts_writes_no_note_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeAccounts(t, fakeReportStore{accounts: closedAccounts(3)}, failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.Empty(t, stderr.String())
}

func Test_accounts_reports_a_failed_stdout_write(t *testing.T) {
	err := executeAccounts(t, fakeReportStore{}, failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}

func Test_accounts_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAccounts(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--json")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_accounts_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr)

	err := cli.Execute(t.Context(), []string{"accounts"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_status_and_accounts_take_no_arguments(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"status", "extra"}, want: "status takes no arguments"},
		{name: "accounts", args: []string{"accounts", "extra"}, want: "accounts takes no arguments"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout bytes.Buffer
			env := failingReportEnv(errStoreRead, &stdout, io.Discard)

			err := cli.Execute(t.Context(), c.args, env)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, usage.Error())
			assert.Empty(t, stdout.String())
		})
	}
}

// leftOutAccounts is a listing whose brokerage holds one unpriced security.
func leftOutAccounts() fakeReportStore {
	return fakeReportStore{accounts: store.AccountList{
		AsOf: asOf,
		Accounts: []store.AccountBalance{
			{
				ID: "acct-1", Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true,
				Balance: big.NewInt(0), Cash: big.NewInt(0), HoldingsValue: big.NewInt(0), BalanceCAD: big.NewInt(0),
			},
		},
		Unvalued: []store.UnvaluedHolding{
			{Date: asOf, AccountID: "acct-1", Account: "Brokerage", SecurityID: "sec-1", Security: "Acme", Currency: new("CAD")},
		},
	}}
}

const leftOutAccountsWarning = `"Brokerage" holds 1 security with no price on or before 2026-09-29, ` +
	`so its balance leaves it out; enter a price in Quicken, then run quarry sync`

func Test_accounts_writes_the_holdings_warnings_to_stderr_after_the_listing(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAccounts(t, leftOutAccounts(), &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, stderrOf([]string{leftOutAccountsWarning}), stderr.String())
}

func Test_accounts_writes_no_holdings_warning_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeAccounts(t, leftOutAccounts(), failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.Empty(t, stderr.String())
}

func classifiedAccounts() store.AccountList {
	row := func(id, name, accountType string) store.AccountBalance {
		return store.AccountBalance{
			ID: id, Name: name, Type: accountType, Currency: "CAD", Active: true,
			Balance: big.NewInt(0), Cash: big.NewInt(0),
		}
	}
	return store.AccountList{AsOf: asOf, Accounts: []store.AccountBalance{
		row("acct-1", "Growth", "brokerage"),
		row("acct-2", "Income", "brokerage"),
		row("acct-3", "Unlisted", "brokerage"),
		row("acct-4", "Chequing", "chequing"),
		row("acct-5", "Savings", "chequing"),
	}}
}

func executeClassified(t *testing.T, stdout *bytes.Buffer, args ...string) error {
	t.Helper()

	return executeClassifiedList(t, stdout, classifiedAccounts(), args...)
}

func executeClassifiedList(t *testing.T, stdout *bytes.Buffer, list store.AccountList, args ...string) error {
	t.Helper()
	env := reportEnv(fakeReportStore{accounts: list}, stdout, &bytes.Buffer{}, withConfig(config.Config{
		Currency:      money.CAD,
		Registered:    []string{"acct-1", "acct-5"},
		NonRegistered: []string{"acct-2"},
	}))
	return cli.Execute(t.Context(), append([]string{"accounts", "--currency", "native"}, args...), env)
}

func Test_accounts_show_their_classification(t *testing.T) {
	t.Run("the Status column says registered or unclassified", func(t *testing.T) {
		var stdout bytes.Buffer

		err := executeClassified(t, &stdout)

		require.NoError(t, err)
		assert.Equal(t, ""+
			"Account   Type       Currency  Balance  Status\n"+
			"Growth    brokerage  CAD          0.00  registered\n"+
			"Income    brokerage  CAD          0.00\n"+
			"Unlisted  brokerage  CAD          0.00  unclassified\n"+
			"Chequing  chequing   CAD          0.00\n"+
			"Savings   chequing   CAD          0.00  registered\n", stdout.String())
	})

	t.Run("--json carries registered after linked_tracking as true, false or null", func(t *testing.T) {
		var stdout bytes.Buffer

		err := executeClassified(t, &stdout, "--json")

		require.NoError(t, err)
		var got struct {
			Accounts []map[string]json.RawMessage `json:"accounts"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
		registered := make([]string, len(got.Accounts))
		for i, a := range got.Accounts {
			registered[i] = string(a["registered"])
		}
		assert.Equal(t, []string{"true", "false", "null", "null", "true"}, registered)
		doc := stdout.String()
		assert.Less(t, strings.Index(doc, `"linked_tracking"`), strings.Index(doc, `"registered"`))
		assert.Less(t, strings.Index(doc, `"registered"`), strings.Index(doc, `"balance"`))
	})
}

// closedUnlistedList holds an open registered brokerage and a closed brokerage no list names.
func closedUnlistedList() store.AccountList {
	row := func(id, name string, closed bool) store.AccountBalance {
		return store.AccountBalance{
			ID: id, Name: name, Type: "brokerage", Currency: "CAD", Active: true, Closed: closed,
			Balance: big.NewInt(0), Cash: big.NewInt(0),
		}
	}

	return store.AccountList{AsOf: asOf, Accounts: []store.AccountBalance{row("acct-1", "Growth", false), row("acct-9", "Old", true)}}
}

func Test_accounts_list_a_closed_unclassified_account_only_with_all(t *testing.T) {
	var without, with bytes.Buffer

	require.NoError(t, executeClassifiedList(t, &without, closedUnlistedList()))
	require.NoError(t, executeClassifiedList(t, &with, closedUnlistedList(), "--all"))

	assert.Equal(t, ""+
		"Account  Type       Currency  Balance  Status\n"+
		"Growth   brokerage  CAD          0.00  registered\n", without.String())
	assert.Equal(t, ""+
		"Account  Type       Currency  Balance  Status\n"+
		"Growth   brokerage  CAD          0.00  registered\n"+
		"Old      brokerage  CAD          0.00  closed, unclassified\n", with.String())
}

func Test_accounts_json_gives_a_closed_unclassified_account_a_null_registered_with_all(t *testing.T) {
	var stdout bytes.Buffer

	err := executeClassifiedList(t, &stdout, closedUnlistedList(), "--all", "--json")

	require.NoError(t, err)
	var got struct {
		Accounts []struct {
			Registered *bool `json:"registered"`
		} `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.Len(t, got.Accounts, 2)
	assert.True(t, *got.Accounts[0].Registered)
	assert.Nil(t, got.Accounts[1].Registered)
}
