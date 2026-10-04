package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
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
		list.Accounts = append(list.Accounts, store.AccountBalance{ID: "acct", Name: "Old", Type: "chequing", Currency: "CAD", Closed: true, Active: true, Balance: 0})
	}
	return list
}

func executeAccounts(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"accounts"}, args...), env)
}

func Test_accounts_all_closed_note(t *testing.T) {
	const header = "Account  Type  Currency  Balance  Status\n"
	open := store.AccountBalance{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true, Balance: 100}
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
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     &stdout, Stderr: &stderr,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

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
			env := cli.Env{
				LoadConfig: cadConfig,
				Stdout:     &stdout, Stderr: io.Discard,
				NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
			}

			err := cli.Execute(t.Context(), c.args, env)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, usage.Error())
			assert.Empty(t, stdout.String())
		})
	}
}
