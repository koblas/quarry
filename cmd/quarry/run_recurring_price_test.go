package main

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_recurring_lists_price_changes_both_ways_from_first_to_latest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cents := slices.Concat(slices.Repeat([]int64{999}, 8), slices.Repeat([]int64{1199}, 8), slices.Repeat([]int64{1099}, 8))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		monthlySeries("Netflix.com", 2024, time.October, cents...)...))
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer

	textCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&textOut, &textErr))
	jsonCode := runWith(context.Background(), []string{"recurring", "--since", "2000", "--json"}, spendEnv(&jsonOut, &jsonErr))

	require.Equal(t, 0, textCode, textErr.String())
	require.Equal(t, 0, jsonCode, jsonErr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts",
		[]string{"Netflix.com", "CAD", "month", "10.99", "131.88", "2024-10-12", "2026-09-12", "active, new", "2: 9.99 -> 10.99 (+10.0%)"},
		[]string{"Total", "CAD", "", "", "131.88", "", "", "", ""}),
		textOut.String())
	doc := decodeRecurringJSON(t, jsonOut.String())
	require.Len(t, doc.Series, 1)
	assert.Equal(t, []recurringPriceChangeJSON{
		{Date: "2025-06-12", From: "9.99", To: "11.99", ChangePct: 20.0},
		{Date: "2026-02-12", From: "11.99", To: "10.99", ChangePct: -8.3},
	}, doc.Series[0].PriceChanges)
}

func Test_run_recurring_leaves_out_a_bill_whose_amount_changes_most_months(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		monthlySeries("Hydro", 2026, time.February, 1000, 1200, 1000, 1200, 1000, 1200, 1000, 1200)...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts"), stdout.String())
}
