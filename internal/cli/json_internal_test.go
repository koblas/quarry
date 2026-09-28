// White-box: jsonMoney and renderJSON's list-emptiness behaviour are
// unexported rules best driven directly, rather than through a full
// command run for every case.
package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_jsonMoney(t *testing.T) {
	cases := []struct {
		name  string
		cents int64
		want  string
	}{
		{name: "zero", cents: 0, want: "0.00"},
		{name: "negative with a zero integer part", cents: -1, want: "-0.01"},
		{name: "negative below one unit", cents: -50, want: "-0.50"},
		{name: "positive with both parts", cents: 120417, want: "1204.17"},
		{name: "large negative has no thousands grouping", cents: -100000000, want: "-1000000.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, jsonMoney(c.cents))
		})
	}
}

// The nil arm proves absent lists encode as []; the populated control arm
// proves the same fields still carry real content, not a hardcoded [].
func Test_renderJSON_encodes_absent_lists_as_empty_arrays(t *testing.T) {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	nilResult := store.Result{Path: "/store", Built: true}
	populatedResult := store.Result{
		Path: "/store", Built: true,
		Validation: store.Validation{
			Balances: store.BalanceCheck{
				NeverReconciled: []store.Account{{ID: "acct-1", Name: "Wallet", Currency: "CAD"}},
				Mismatched:      []store.BalanceMismatch{{ID: "acct-2", Name: "Chequing", StatementDate: day}},
			},
			Splits: store.SplitCheck{
				Mismatched: []store.SplitMismatch{{ID: "txn-1", Account: "Chequing", Date: day}},
			},
			Transfers: store.TransferCheck{
				OneSided: []store.OneSidedTransfer{{ID: "xfer-1", Account: "Chequing", Date: day}},
			},
		},
	}

	nilData, err := renderJSON(snapshot.Outcome{Store: &nilResult})
	require.NoError(t, err)
	populatedData, err := renderJSON(snapshot.Outcome{Store: &populatedResult})
	require.NoError(t, err)

	nilBalances, nilSplits, nilOneSided := storeLists(t, nilData)
	assert.Equal(t, []any{}, nilBalances["mismatched"])
	assert.Equal(t, []any{}, nilBalances["never_reconciled"])
	assert.Equal(t, []any{}, nilSplits)
	assert.Equal(t, []any{}, nilOneSided)

	populatedBalances, populatedSplits, populatedOneSided := storeLists(t, populatedData)
	assert.Len(t, populatedBalances["mismatched"], 1)
	assert.Len(t, populatedBalances["never_reconciled"], 1)
	assert.Len(t, populatedSplits, 1)
	assert.Len(t, populatedOneSided, 1)
}

// storeLists decodes data's list fields as generic values, so a JSON null decodes distinguishably from an empty array.
func storeLists(t *testing.T, data []byte) (balances map[string]any, splitsMismatched, oneSided any) {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	store, ok := doc["store"].(map[string]any)
	require.True(t, ok)
	balances, ok = store["balances"].(map[string]any)
	require.True(t, ok)
	splits, ok := store["splits"].(map[string]any)
	require.True(t, ok)
	transfers, ok := store["transfers"].(map[string]any)
	require.True(t, ok)
	return balances, splits["mismatched"], transfers["one_sided"]
}
