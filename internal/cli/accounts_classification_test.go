package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classifiedAccounts() store.AccountList {
	row := func(id, name, accountType string) store.AccountBalance {
		return store.AccountBalance{
			Account: store.Account{ID: id, Name: name, Type: accountType, Currency: "CAD", Active: true},
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
	env := cli.Env{
		LoadConfig: func(string) (config.Config, error) {
			return config.Config{
				Currency:      money.CAD,
				Registered:    []string{"acct-1", "acct-5"},
				NonRegistered: []string{"acct-2"},
			}, nil
		},
		Stdout: stdout, Stderr: &bytes.Buffer{},
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{accounts: classifiedAccounts()})), nil
		},
	}
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
