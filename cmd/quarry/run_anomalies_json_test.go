package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomalyJSON is one entry of the anomalies document's "anomalies".
type anomalyJSON struct {
	TransactionID string  `json:"transaction_id"`
	Date          string  `json:"date"`
	AccountID     string  `json:"account_id"`
	Account       string  `json:"account"`
	Currency      string  `json:"currency"`
	Payee         *string `json:"payee"`
	Category      *string `json:"category"`
	Amount        string  `json:"amount"`
	Baseline      string  `json:"baseline"`
	Usual         string  `json:"usual"`
	// NativeCurrency, NativeAmount and NativeUsual are the charge's own currency, Amount and Usual.
	NativeCurrency string  `json:"native_currency"`
	NativeAmount   string  `json:"native_amount"`
	NativeUsual    string  `json:"native_usual"`
	Earlier        int     `json:"earlier"`
	Times          float64 `json:"times"`
}

// anomaliesJSONDoc is quarry anomalies --json's stdout.
type anomaliesJSONDoc struct {
	Since         string            `json:"since"`
	Until         string            `json:"until"`
	Currency      string            `json:"currency"`
	AccountFilter []recurringIDName `json:"account_filter"`
	Anomalies     []anomalyJSON     `json:"anomalies"`
	Checked       int               `json:"checked"`
	NotJudged     int               `json:"not_judged"`
	Warnings      []string          `json:"warnings"`
}

// decodeAnomaliesJSON reads stdout as the anomalies document, refusing keys the document does not rule.
func decodeAnomaliesJSON(t *testing.T, stdout string) anomaliesJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc anomaliesJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

func Test_run_anomalies_json_returns_the_anomalies_document(t *testing.T) {
	home := newHome(t)
	charges := make([]chargeTxn, 0, 7)
	for i, cents := range []int64{9000, 9300, 9605, 9900, 10200} {
		charges = append(charges, groceryCharge("Bell Canada", day(2025, time.March, 3+7*i), cents))
	}
	charges = append(charges, groceryCharge("Bell Canada", day(2026, time.March, 2), 41200))
	charges = append(charges, chargeTxn{
		id: "tool-shed", account: "acct-cad", payee: "Tool Shed", currency: "CAD", day: day(2026, time.June, 9),
		splits: []chargeSplit{{cents: -15000}},
	})
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesJSONDoc{
		Since:         "2026-01-01",
		Until:         "2026-09-29",
		Currency:      "CAD",
		AccountFilter: []recurringIDName{},
		Anomalies: []anomalyJSON{{
			TransactionID:  "txn-Bell Canada2026-03-02",
			Date:           "2026-03-02",
			AccountID:      "acct-cad",
			Account:        "Chequing",
			Currency:       "CAD",
			Payee:          new("Bell Canada"),
			Category:       new("Food:Groceries"),
			Amount:         "412.00",
			Baseline:       "payee",
			Usual:          "96.05",
			NativeCurrency: "CAD",
			NativeAmount:   "412.00",
			NativeUsual:    "96.05",
			Earlier:        5,
			Times:          4.3,
		}},
		Checked:   2,
		NotJudged: 1,
		Warnings:  []string{},
	}, decodeAnomaliesJSON(t, stdout.String()))
}
