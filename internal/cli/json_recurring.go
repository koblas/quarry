package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderRecurringJSON renders r as recurring's --json document with warnings.
func renderRecurringJSON(r report.Recurring, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewRecurring(r, warnings))
}
