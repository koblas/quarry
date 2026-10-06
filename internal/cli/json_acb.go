package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderACBJSON renders a as acb's --json document with warnings.
func renderACBJSON(a report.ACB, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewACB(a, warnings))
}
