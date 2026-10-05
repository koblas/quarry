package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderNetWorthJSON renders n as networth's --json document with warnings.
func renderNetWorthJSON(n report.NetWorth, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewNetWorth(n, warnings))
}
