package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// defaultSQLLimit is how many rows sql prints unless --limit says otherwise.
const defaultSQLLimit = 500

// newSQLCommand builds sql: one query run verbatim against the store, at most
// --limit rows of its result printed as a table, or as JSON when *jsonOut is set.
func newSQLCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "sql <query>",
		Short: "Run a read-only SQL query against quarry's store",
		Long: `Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions. A query too large for memory may spill to a temporary
directory beside the store; quarry removes it when it exits.

Pass the query as one quoted argument, or - to read it from stdin. A query
that starts with - (such as a -- comment) goes after --:

  quarry sql -- "-- monthly totals
  SELECT ..."

Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. Transfers between your own accounts are in the transfers table
and splits.transfer_account_id, never in a category kind. List the tables
and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set); when there are more,
quarry says so on stderr. --limit 0 prints every row.`,
		Example: `  quarry sql "SELECT name, currency FROM accounts WHERE NOT closed"
  quarry sql --limit 0 --json - < monthly.sql`,
		Args: func(_ *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				return errSQLNeedsQuery
			case len(args) > 1:
				return errSQLTakesOneQuery
			case limit < 0:
				return errSQLNegativeLimit
			case strings.TrimSpace(args[0]) == "":
				return errSQLNeedsQuery
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if query == readQueryFromStdin {
				// Before newReport: an empty stdin is a usage error, which never needs $HOME.
				var err error
				if query, err = readStdinQuery(cmd.Context(), cmd.InOrStdin()); err != nil {
					return err
				}
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			result, err := srv.Query(cmd.Context(), query, limit)
			if err != nil {
				return &runtimeError{err: queryFailure(err)}
			}

			warnings := []string{}
			if result.Truncated {
				warnings = append(warnings, truncationNote(limit))
			}

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderSQLJSON(result, limit, warnings) },
				func() string { return renderSQLTable(result.QueryResult) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "quarry: warning: ", warnings)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", defaultSQLLimit, "print at most `n` rows (0 prints every row)")
	return cmd
}

// readQueryFromStdin is the query argument that reads the query from stdin.
const readQueryFromStdin = "-"

// errSQLNeedsQuery refuses a missing or blank query, stdin's included.
var errSQLNeedsQuery = UsageError{msg: "sql needs a query; pass it as one quoted argument, or - to read it from stdin"}

// errSQLTakesOneQuery refuses more than one query argument.
var errSQLTakesOneQuery = UsageError{msg: "sql takes one query; quote it as one argument"}

// errSQLNegativeLimit refuses a --limit below zero.
var errSQLNegativeLimit = UsageError{msg: "--limit must be 0 or more; 0 prints every row"}

// readStdinQuery reads all of in as the query, verbatim. It refuses an
// interrupt, then a read fault, then a blank query, in that order.
func readStdinQuery(ctx context.Context, in io.Reader) (string, error) {
	type read struct {
		query []byte
		err   error
	}
	done := make(chan read, 1)
	go func() {
		query, err := io.ReadAll(in)
		done <- read{query: query, err: err}
	}()

	var got read
	select {
	case <-ctx.Done():
	case got = <-done:
	}
	// A read that finishes as the interrupt lands still reports the interrupt.
	if err := ctx.Err(); err != nil {
		return "", &runtimeError{err: queryFailure(store.Interrupted(err))}
	}
	if got.err != nil {
		return "", &runtimeError{err: &refusalError{text: "cannot read the query from stdin: " + osReason(got.err), cause: got.err}}
	}
	if strings.TrimSpace(string(got.query)) == "" {
		return "", errSQLNeedsQuery
	}
	return string(got.query), nil
}

// osReason is err's OS-supplied reason: a path error's own cause, else the
// first line of err, or "unknown error" when that is empty.
func osReason(err error) string {
	reason, _, _ := strings.Cut(err.Error(), "\n")
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		reason = pathErr.Err.Error()
	}
	if reason == "" {
		return "unknown error"
	}
	return reason
}

// truncationNote is the warning that sql cut its rows at limit.
func truncationNote(limit int) string {
	return "showing the first " + humanize.Count(limit, "row", "rows") + "; the query returned more; pass --limit 0 to print every row"
}

// queryFailure is the error sql reports for a failed query: the ruled
// refusal copy for each store refusal, otherwise err unchanged.
func queryFailure(err error) error {
	if unprintable, ok := errors.AsType[*store.UnprintableValueError](err); ok {
		return fmt.Errorf("%w; cast it in the query, e.g. CAST(%s AS VARCHAR)", unprintable, unprintable.Column)
	}
	if queryErr, ok := errors.AsType[*store.QueryError](err); ok {
		return &refusalError{text: "query failed: " + queryErr.Reason, cause: err}
	}
	switch {
	case errors.Is(err, store.ErrEmptyQuery):
		return errSQLNeedsQuery
	case errors.Is(err, store.ErrReadOnlyQuery):
		return &refusalError{text: "quarry sql only reads the store; change the data in Quicken and run quarry sync", cause: err}
	case errors.Is(err, store.ErrExternalAccess):
		return &refusalError{text: "quarry sql reads only quarry's store; other files, databases and extensions are turned off", cause: err}
	case errors.Is(err, store.ErrQueryInterrupted):
		return &refusalError{text: "query interrupted", cause: err}
	}
	return err
}

// refusalError is a refusal's user copy over the error that caused it.
type refusalError struct {
	text  string
	cause error
}

func (e *refusalError) Error() string { return e.text }
func (e *refusalError) Unwrap() error { return e.cause }
