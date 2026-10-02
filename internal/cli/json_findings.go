package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderFindingsJSON renders listing as findings's --json document for view with warnings as given;
// findings and warnings are [] rather than null when empty.
func renderFindingsJSON(listing report.FindingsListing, view findingsView, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewFindingsList(listing, view.status, view.typ, warnings))
}
