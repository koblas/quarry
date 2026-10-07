package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_holdings_json_carries_account_and_security_names_as_stored(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.Accounts[0].Name = "Broker\nage"
	rows.Securities[0].Name = "Acme\tCorp\r\n"
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--json", "--account", "acct-cad"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Holdings []struct {
			Account  string `json:"account"`
			Security string `json:"security"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Holdings, 1)
	assert.Equal(t, "Broker\nage", doc.Holdings[0].Account)
	assert.Equal(t, "Acme\tCorp\r\n", doc.Holdings[0].Security)
}
