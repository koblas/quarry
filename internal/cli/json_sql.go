package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// renderSQLJSON renders result as sql's --json document: row_count counts its
// rows, limit echoes --limit, and empty columns and rows are [], never null.
func renderSQLJSON(result report.QueryResult, limit int, warnings []string) ([]byte, error) {
	return marshalDocument(document.NewSQL(result, limit, warnings))
}
