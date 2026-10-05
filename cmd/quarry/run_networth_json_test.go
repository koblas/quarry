package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_networth_json_keeps_a_zero_balance_row_and_counts_a_closed_account_but_not_one_left_out_of_reports(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, map[string]any{
		"as_of": "2026-03-12", "since": nil, "until": nil, "currency": "CAD",
		"dates": []any{map[string]any{
			"date": "2026-03-12",
			"balances": []any{
				map[string]any{"type": "brokerage", "currency": "USD", "balance": "920.00", "converted_balance": "1251.20"},
				map[string]any{"type": "chequing", "currency": "CAD", "balance": "1025.00", "converted_balance": "1025.00"},
				map[string]any{"type": "chequing", "currency": "USD", "balance": "800.00", "converted_balance": "1088.00"},
				map[string]any{"type": "credit_card", "currency": "CAD", "balance": "-250.50", "converted_balance": "-250.50"},
				map[string]any{"type": "savings", "currency": "CAD", "balance": "0.00", "converted_balance": "0.00"},
			},
			"totals": []any{map[string]any{"currency": "CAD", "value": "3113.70"}},
		}},
		"warnings": []any{},
	}, got)
}

func Test_run_networth_json_lists_each_currency_total_and_no_converted_balance_in_native_mode(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--json", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var got struct {
		Currency string `json:"currency"`
		Dates    []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "native", got.Currency)
	assert.Equal(t, []map[string]any{
		{"currency": "CAD", "value": "774.50"}, {"currency": "USD", "value": "1720.00"},
	}, got.Dates[0].Totals)
	assert.Len(t, got.Dates[0].Balances, 5)
	assert.Equal(t, []any{nil, nil, nil, nil, nil}, convertedBalances(got.Dates[0].Balances))
}

func convertedBalances(balances []map[string]any) []any {
	converted := make([]any, len(balances))
	for i, b := range balances {
		converted[i] = b["converted_balance"]
	}
	return converted
}
