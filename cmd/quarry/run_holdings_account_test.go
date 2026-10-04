package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const chequingWarning = "quarry: warning: account \"Chequing\" is not a brokerage or retirement account, so it has no holdings\n"

// holdingsAccountDoc reads back the holdings --json keys the account filter tests pin.
type holdingsAccountDoc struct {
	AccountFilter []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"account_filter"`
	Warnings []string `json:"warnings"`
}

// seedHoldingsAccounts stores holdingsRows with Chequing, an empty brokerage and two accounts named Visa added.
func seedHoldingsAccounts(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Accounts = append(rows.Accounts,
		chequingAccount("acct-chq", 4),
		store.Account{ID: "acct-empty", SourceID: 5, Name: "Empty", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		store.Account{ID: "acct-977", SourceID: 6, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
		store.Account{ID: "acct-812", SourceID: 7, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func runHoldingsAccounts(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"holdings"}, args...), spendEnvAt(&stdout, &stderr, holdingsClock()))

	return exitCode, stdout.String(), stderr.String()
}

// holdingsAccountLine is one table line sized to the Brokerage-only listing: Acme Corp (ACME) is its widest security.
func holdingsAccountLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return fmt.Sprintf("%-9s  %-16s  %6s  %5s  %-10s  %-8s  %9s  %9s\n",
		account, security, shares, price, pricedOn, currency, value, in)
}

func Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Accounts = append(rows.Accounts, chequingAccount("acct-chq", 4))
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--account", "Brokerage", "--account", "Chequing"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in Brokerage, Chequing, amounts in CAD; cash not included\n\n"+
		holdingsAccountLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsAccountLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsAccountLine("Total", "", "", "", "", "", "", "37,704.00"),
		stdout.String())
	assert.Equal(t, chequingWarning, stderr.String())
}

func Test_run_holdings_refuses_an_account_it_cannot_pick_with_nothing_on_stdout(t *testing.T) {
	const (
		unknown   = "quarry: no account named \"Chequeing\"; run quarry accounts --all to list them\n"
		empty     = "quarry: no account named \"\"; run quarry accounts --all to list them\n"
		ambiguous = "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n"
	)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"an unknown name", []string{"--account", "Chequeing"}, unknown},
		{"an empty argument", []string{"--account", ""}, empty},
		{"an ambiguous name", []string{"--account", "Visa"}, ambiguous},
		{"an unknown name as JSON", []string{"--account", "Chequeing", "--json"}, unknown},
		{"an empty argument as JSON", []string{"--account", "", "--json"}, empty},
		{"an ambiguous name as JSON", []string{"--account", "Visa", "--json"}, ambiguous},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedHoldingsAccounts(t)

			exitCode, stdout, stderr := runHoldingsAccounts(t, c.args...)

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.want, stderr)
			assert.Empty(t, stdout)
		})
	}
}

func Test_run_holdings_of_only_a_non_investment_account_prints_both_warnings_in_order(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--account", "Chequing")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in Chequing, amounts in CAD; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n", stdout)
	assert.Equal(t, chequingWarning+namedNothingWarning, stderr)
}

func Test_run_holdings_of_an_investment_account_with_nothing_held_warns_only_that_nothing_is_held(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, _, stderr := runHoldingsAccounts(t, "--account", "Empty")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, namedNothingWarning, stderr)
}

func Test_run_holdings_json_lists_the_named_accounts_and_the_warnings_stderr_prints(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"in the reporting currency", []string{"--json"}},
		{"in native amounts", []string{"--json", "--currency", "native"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedHoldingsAccounts(t)

			exitCode, stdout, stderr := runHoldingsAccounts(t, append([]string{"--account", "Chequing", "--account", "Brokerage"}, c.args...)...)

			require.Equal(t, 0, exitCode, stderr)
			var doc holdingsAccountDoc
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			assert.JSONEq(t, `[{"id":"acct-chq","name":"Chequing"},{"id":"acct-cad","name":"Brokerage"}]`, mustJSON(t, doc.AccountFilter))
			assert.Equal(t, []string{"account \"Chequing\" is not a brokerage or retirement account, so it has no holdings"}, doc.Warnings)
			assert.Equal(t, chequingWarning, stderr)
		})
	}
}

func Test_run_holdings_names_an_account_given_by_name_and_by_id_once(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--account", "Brokerage", "--account", "acct-cad", "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc holdingsAccountDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.JSONEq(t, `[{"id":"acct-cad","name":"Brokerage"}]`, mustJSON(t, doc.AccountFilter))
}

func Test_run_holdings_without_an_account_lists_every_account_and_an_empty_account_filter(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Contains(t, stdout, `"account_filter": []`)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}
