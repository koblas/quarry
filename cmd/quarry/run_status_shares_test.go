// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_reports_the_share_check(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, holdingsBundle(t, home))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(),
		"Rows      4 transactions, 4 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 3 investment transactions, 2 securities, 3 prices\n")
	assert.Contains(t, stdout.String(), "Shares    2 holdings match Quicken's share counts\n")
	assert.NotContains(t, stdout.String(), "not imported")

	var jsonOut, jsonErr bytes.Buffer
	exitCode = run(context.Background(), []string{"status", "--json"}, &jsonOut, &jsonErr)

	require.Equal(t, 0, exitCode, jsonErr.String())
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.JSONEq(t, `{"checked":2}`, string(doc["shares"]))
	assert.NotContains(t, doc, "not_imported")
	var storeDoc struct {
		Rows map[string]int `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(doc["store"], &storeDoc))
	assert.Equal(t, 3, storeDoc.Rows["investment_transactions"])
	assert.Equal(t, 2, storeDoc.Rows["securities"])
	assert.Equal(t, 3, storeDoc.Rows["prices"])
}
