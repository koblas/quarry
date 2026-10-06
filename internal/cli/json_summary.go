package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderSummaryJSON renders s, its findings tally and warnings as summary's --json document, encoded like
// sync's: 2-space indent, trailing newline.
func renderSummaryJSON(s report.Summary, findings document.FindingsTally, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewSummary(s, findings, warnings))
}
