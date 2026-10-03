package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	rowCap          = 500
	spendingCapLine = "spending lists the first 500 rows of 501; totals count every row; pass a shorter period or fewer accounts, or query v_spending for the rest"
)

// payeeStore is a store with one charge from each of n distinct payees.
func payeeStore(n int) func(*testing.T, string) {
	return func(t *testing.T, home string) {
		t.Helper()
		charges := make([]chargeTxn, n)
		for i := range charges {
			charges[i] = groceryCharge(fmt.Sprintf("Payee %03d", i), day(2026, time.March, 1+i%28), int64(1000+i))
		}
		replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	}
}

// decodeSpendingBody is a compact spending document, as runBothSurfaces returns it, decoded.
func decodeSpendingBody(t *testing.T, body string) document.Spending {
	t.Helper()
	var doc document.Spending
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	return doc
}

func Test_run_mcp_spending_cuts_a_list_over_500_rows_with_the_cap_warning(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: payeeStore(rowCap + 1), cliArgs: []string{"spend", "--by", "payee"},
		tool: "spending", arguments: map[string]any{"by": "payee"},
	})

	cli, tool := decodeSpendingBody(t, got.cliBody), decodeSpendingBody(t, got.toolBody)

	require.Len(t, cli.Rows, rowCap+1)
	assert.Equal(t, cli.Rows[:rowCap], tool.Rows)
	assert.Equal(t, cli.Totals, tool.Totals)
	assert.Equal(t, spendingCapLine, got.toolWarnings[len(got.toolWarnings)-1])
	assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings[:len(got.toolWarnings)-1])
}

func Test_run_mcp_spending_leaves_a_list_of_500_rows_uncut_and_adds_no_cap_warning(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: payeeStore(rowCap), cliArgs: []string{"spend", "--by", "payee"},
		tool: "spending", arguments: map[string]any{"by": "payee"},
	})

	assert.Equal(t, got.cliBody, got.toolBody)
	assert.Len(t, decodeSpendingBody(t, got.toolBody).Rows, rowCap)
	assert.NotContains(t, got.toolWarnings, spendingCapLine)
	assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
}
