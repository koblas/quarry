package cli_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
