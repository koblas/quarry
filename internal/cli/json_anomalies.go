package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderAnomaliesJSON renders a as anomalies' --json document with warnings.
func renderAnomaliesJSON(a report.Anomalies, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewAnomalies(a, warnings))
}
