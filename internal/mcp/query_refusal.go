package mcp

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/koblas/quarry/internal/report"
)

// The refusals of a query that would change the store or reach outside it.
var (
	errWriteRefused    = errors.New("query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync")
	errExternalRefused = errors.New("query reads only quarry's store; other files, databases and extensions are turned off")
)

// The stderr lines of a query the database rejected or could not print, which carry no reason, type or name of it.
const (
	rejectedLogTail = "; details went to the client only"
	unprintableLog  = "query failed: a result column has a type quarry cannot print" + rejectedLogTail
	unclassifiedSQL = "SQL error"
)

// reasonClass matches the class DuckDB opens a reason with, e.g. "Binder Error".
var reasonClass = regexp.MustCompile(`^[A-Z][A-Za-z]*( [A-Z][A-Za-z]*)* Error$`)

// queryRefusal words err, a failed Server.Query, for the model that wrote the query, and chooses its stderr line.
// Store refusals and failures with no reason of their own pass through with their text unchanged.
func queryRefusal(err error) error {
	failure := report.ClassifyQueryFailure(err)
	switch failure.Kind {
	case report.QueryFailureUnprintable:
		return withLog(fmt.Errorf("%w; cast it in the query, e.g. CAST(%s AS VARCHAR)", failure.Err, failure.Detail), unprintableLog)
	case report.QueryFailureRejected:
		return withLog(fmt.Errorf("query failed: %w", failure.Err), "query failed: "+rejectedClass(failure.Detail)+rejectedLogTail)
	case report.QueryFailureEmpty:
		return verbatim(errBlankSQL)
	case report.QueryFailureReadOnly:
		return verbatim(errWriteRefused)
	case report.QueryFailureExternalAccess:
		return verbatim(errExternalRefused)
	case report.QueryFailureInterrupted:
		// The classified Err drops the context error that err keeps.
		return err
	case report.QueryFailureOther:
		return err
	}
	// unreachable: every QueryFailureKind has a case above and the exhaustive linter fails the build when one is added
	return err
}

// rejectedClass is the class DuckDB's reason opens with, "SQL error" when it does not open with one.
func rejectedClass(reason string) string {
	if class, _, found := strings.Cut(reason, ": "); found && reasonClass.MatchString(class) {
		return class
	}
	return unclassifiedSQL
}
