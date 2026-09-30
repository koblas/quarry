// White-box: no finding type fills category, transactions or splits yet, so the cells that
// carry them are driven directly over a findingItemDocument.
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_findingItemCSVCells_carries_category_and_counts_when_the_item_has_them(t *testing.T) {
	category := "Groceries"
	transactions, splits := 48, 3
	item := findingItemDocument{Date: "2026-08-01", Account: "Chequing", Currency: "CAD", Amount: "-1.00", Category: &category, Transactions: &transactions, Splits: &splits}

	cells := findingItemCSVCells(item)

	assert.Equal(t, csvCell{Text: "Groceries"}, cells[4])
	assert.Equal(t, csvCell{Text: "48"}, cells[7])
	assert.Equal(t, csvCell{Text: "3"}, cells[8])
}
