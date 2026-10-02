// White-box: renderJSON's list-emptiness behaviour is an unexported rule
// best driven directly, rather than through a full command run for every case.
package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	nilData, err := renderJSON(snapshot.Outcome{Store: &nilResult}, nil)
	require.NoError(t, err)
	populatedData, err := renderJSON(snapshot.Outcome{Store: &populatedResult}, nil)
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
func storeLists(t *testing.T, data []byte) (map[string]any, any, any) {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	store, ok := doc["store"].(map[string]any)
	require.True(t, ok)
	balances, ok := store["balances"].(map[string]any)
	require.True(t, ok)
	splits, ok := store["splits"].(map[string]any)
	require.True(t, ok)
	transfers, ok := store["transfers"].(map[string]any)
	require.True(t, ok)
	return balances, splits["mismatched"], transfers["one_sided"]
}

// Nil lists in a Pruned must encode as [], and no Pruned at all as null: the key is always present.
func Test_renderJSON_encodes_pruned_lists_as_empty_arrays_and_no_pruned_as_null(t *testing.T) {
	built := store.Result{Path: "/store", Built: true}

	withPruned, err := renderJSON(snapshot.Outcome{Store: &built, Pruned: &snapshot.Pruned{Keep: 12}}, nil)
	require.NoError(t, err)
	without, err := renderJSON(snapshot.Outcome{Store: &built}, nil)
	require.NoError(t, err)

	var doc, bare map[string]any
	require.NoError(t, json.Unmarshal(withPruned, &doc))
	require.NoError(t, json.Unmarshal(without, &bare))
	assert.Equal(t, map[string]any{"keep": float64(12), "deleted": []any{}, "failed": []any{}}, doc["pruned"])
	assert.Contains(t, bare, "pruned")
	assert.Nil(t, bare["pruned"])
}

func Test_renderJSON_encodes_the_five_finding_counts_for_a_built_store(t *testing.T) {
	built := store.Result{Path: "/store", Built: true, Findings: finding.Counts{Open: 5, Ignored: 4, Fixed: 3, New: 2, NewlyFixed: 1}}

	data, err := renderJSON(snapshot.Outcome{Store: &built}, nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"open": float64(5), "ignored": float64(4), "fixed": float64(3), "new": float64(2), "newly_fixed": float64(1)}, storeField(t, data, "findings"))
}

func Test_renderJSON_encodes_findings_as_null_when_the_store_was_not_built(t *testing.T) {
	unbuilt := store.Result{Path: "/store", Findings: finding.Counts{Open: 5}}

	data, err := renderJSON(snapshot.Outcome{Store: &unbuilt}, nil)

	require.NoError(t, err)
	assert.Nil(t, storeField(t, data, "findings"))
	assert.Contains(t, string(data), `"findings": null`)
}

func Test_renderJSON_encodes_no_store_as_null_when_the_import_was_not_attempted(t *testing.T) {
	data, err := renderJSON(snapshot.Outcome{}, nil)

	require.NoError(t, err)
	assert.Contains(t, string(data), `"store": null`)
}

// storeField decodes the "store" object of data and returns its field key.
func storeField(t *testing.T, data []byte, key string) any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	store, ok := doc["store"].(map[string]any)
	require.True(t, ok)
	return store[key]
}

func Test_renderJSON_encodes_the_rates_a_build_stored(t *testing.T) {
	built := store.Result{Path: "/store", Built: true, Rates: store.RatesSummary{
		First: time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Added: 12, FetchError: "unreachable",
	}}

	data, err := renderJSON(snapshot.Outcome{Store: &built}, nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"first": "2005-03-01", "last": "2026-03-01", "added": float64(12), "fetch_error": "unreachable"}, storeField(t, data, "rates"))
}

func Test_renderJSON_encodes_no_rates_as_nulls_and_an_unbuilt_store_as_null(t *testing.T) {
	built := store.Result{Path: "/store", Built: true}
	unbuilt := store.Result{Path: "/store", Rates: store.RatesSummary{Added: 3}}

	builtData, err := renderJSON(snapshot.Outcome{Store: &built}, nil)
	require.NoError(t, err)
	unbuiltData, err := renderJSON(snapshot.Outcome{Store: &unbuilt}, nil)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"first": nil, "last": nil, "added": float64(0), "fetch_error": nil}, storeField(t, builtData, "rates"))
	assert.Nil(t, storeField(t, unbuiltData, "rates"))
	assert.Contains(t, string(unbuiltData), `"rates": null`)
}

func Test_renderJSON_ends_the_store_object_with_rates(t *testing.T) {
	built := store.Result{Path: "/store", Built: true}

	data, err := renderJSON(snapshot.Outcome{Store: &built}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"path", "built", "rows", "balances", "splits", "transfers", "findings", "not_imported", "rates"}, storeKeys(t, data))
}

// storeKeys lists the keys of data's "store" object in document order.
func storeKeys(t *testing.T, data []byte) []string {
	t.Helper()
	var doc struct {
		Store json.RawMessage `json:"store"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	dec := json.NewDecoder(bytes.NewReader(doc.Store))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, key.(string)) //nolint:forcetypeassert // an object's keys are strings
		var skip json.RawMessage
		require.NoError(t, dec.Decode(&skip))
	}
	return keys
}
