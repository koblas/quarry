package cli

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderAccountsJSON_renders_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	huge := cents("99999999999999999800000000")
	list := report.AccountListing{
		AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Currency: money.CAD,
		Accounts: []store.AccountBalance{{
			ID: "acct-brk", Type: "brokerage", Currency: "CAD", Balance: huge, Cash: big.NewInt(0), HoldingsValue: huge, BalanceCAD: huge,
		}},
	}

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	var doc struct {
		Accounts []struct {
			Balance          string `json:"balance"`
			Cash             string `json:"cash"`
			HoldingsValue    string `json:"holdings_value"`
			ConvertedBalance string `json:"converted_balance"`
		} `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(got, &doc))
	require.Len(t, doc.Accounts, 1)
	assert.Equal(t, []string{"999999999999999998000000.00", "0.00", "999999999999999998000000.00", "999999999999999998000000.00"},
		[]string{doc.Accounts[0].Balance, doc.Accounts[0].Cash, doc.Accounts[0].HoldingsValue, doc.Accounts[0].ConvertedBalance})
}
