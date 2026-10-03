package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderSpendingJSON renders s as spend's --json document with warnings.
func renderSpendingJSON(s report.Spending, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewSpending(s, warnings))
}
