package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderHoldingsJSON renders h as holdings's --json document with warnings.
func renderHoldingsJSON(h report.Holdings, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewHoldings(h, warnings))
}
