package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomaliesHeader is the header cells of the anomalies table.
var anomaliesHeader = []string{"Date", "Account", "Payee", "Category", "Amount", "Usual", "Times", "Compared with"}

// anomaliesRightAligned are the table's columns of numbers, which pad on the left.
var anomaliesRightAligned = map[int]bool{4: true, 5: true, 6: true}

// anomaliesTable is the anomalies table under caption followed by a blank line and footer: every cell
// but the last padded to its column's widest cell, two-space gaps, numbers right-aligned, trailing spaces trimmed.
func anomaliesTable(caption, footer string, rows ...[]string) string {
	rows = append([][]string{anomaliesHeader}, rows...)
	widths := make([]int, len(anomaliesHeader)-1)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], len(row[i]))
		}
	}
	var b strings.Builder
	b.WriteString(caption + "\n\n")
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			switch {
			case i == len(widths):
				cells[i] = cell
			case anomaliesRightAligned[i]:
				cells[i] = fmt.Sprintf("%*s", widths[i], cell)
			default:
				cells[i] = fmt.Sprintf("%-*s", widths[i], cell)
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
	b.WriteString("\n" + footer + "\n")
	return b.String()
}

func Test_run_anomalies_lists_a_charge_over_twice_the_payees_usual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{9000, 9300, 9605, 9900, 10200} {
		charges = append(charges, groceryCharge("Bell Canada", day(2025, time.March, 3+7*i), cents))
	}
	charges = append(charges, groceryCharge("Bell Canada", day(2026, time.March, 2), 41200))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
		[]string{"2026-03-02", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"}),
		stdout.String())
}
