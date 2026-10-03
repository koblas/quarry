package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderCashFlowJSON renders c as cashflow's --json document with warnings.
func renderCashFlowJSON(c report.CashFlow, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewCashFlow(c, warnings))
}
