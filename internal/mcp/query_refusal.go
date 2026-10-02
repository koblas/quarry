package mcp

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/report"
)

// The refusals of a query that would change the store or reach outside it.
var (
	errWriteRefused    = errors.New("query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync")
	errExternalRefused = errors.New("query reads only quarry's store; other files, databases and extensions are turned off")
)

// queryRefusal words err, a failed Server.Query, for the model that wrote the query. Store
// refusals and failures with no reason of their own pass through with their text unchanged.
func queryRefusal(err error) error {
	failure := report.ClassifyQueryFailure(err)
	switch failure.Kind {
	case report.QueryFailureUnprintable:
		return fmt.Errorf("%w; cast it in the query, e.g. CAST(%s AS VARCHAR)", failure.Err, failure.Detail)
	case report.QueryFailureRejected:
		return fmt.Errorf("query failed: %w", failure.Err)
	case report.QueryFailureEmpty:
		return errBlankSQL
	case report.QueryFailureReadOnly:
		return errWriteRefused
	case report.QueryFailureExternalAccess:
		return errExternalRefused
	case report.QueryFailureInterrupted:
		// The classified Err drops the context error, which handler needs to tell a deadline from a cancel.
		return err
	case report.QueryFailureOther:
		return err
	}
	// unreachable: every QueryFailureKind has a case above and the exhaustive linter fails the build when one is added
	return err
}
