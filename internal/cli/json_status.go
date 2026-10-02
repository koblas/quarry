package cli

import (
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// renderStatusJSON renders st, its findings tally and warnings as status's --json document,
// encoded like sync's: 2-space indent, trailing newline.
func renderStatusJSON(st store.Status, findings document.FindingsTally, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewStatus(st, findings, warnings))
}
