package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/osreason"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// defaultSQLLimit is how many rows sql prints unless --limit says otherwise; --csv prints every row.
const defaultSQLLimit = 500

// newSQLCommand builds sql: one query run verbatim against the store, at most
// --limit rows of its result printed as a table, as JSON when *jsonOut is
// set, or as CSV with --csv.
func newSQLCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
	var limit int
	var csvOut bool
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

`+report.SQLConventions+`

findings holds what sync found to clean up in Quicken, and finding_items
the transactions, splits, payees or categories each one is about;
fixed_at is set once a finding is no longer found. Which findings you
ignored is set in the config file, not the store: quarry findings shows
each one's status.

List the tables and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set, every row with --csv);
when there are more, quarry says so on stderr. --limit 0 prints every row.
With --csv, an empty field is NULL and "" is an empty string, except in a
one-column result, where NULL is also written as "" so no row is blank.`,
		Example: `  quarry sql "SELECT name, currency FROM accounts WHERE NOT closed"
  quarry sql --limit 0 --json - < monthly.sql
  quarry sql --csv "SELECT * FROM transactions" > transactions.csv`,
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
			case csvOut && *jsonOut:
				return errCSVAndJSON
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

			rows := rowLimit(cmd, limit, csvOut)
			result, err := srv.Query(cmd.Context(), query, rows)
			if err != nil {
				return &runtimeError{err: queryFailure(err)}
			}

			warnings := []string{}
			if result.Truncated {
				warnings = append(warnings, truncationNote(rows))
			}

			renderText := func() string { return renderSQLTable(result.QueryResult) }
			if csvOut {
				renderText = func() string { return renderSQLCSV(result.QueryResult) }
			}
			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderSQLJSON(result, rows, warnings) },
				renderText)
			if err != nil {
				return err
			}
			return emit(cmd, out, "quarry: warning: ", warnings)
		},
	}
	cmd.Flags().BoolVar(&csvOut, "csv", false, "print the rows as CSV, with a header line")
	// The flag's own default is 0 so help prints no "(default ...)" beside the ruled text; rowLimit applies 500.
	cmd.Flags().IntVar(&limit, "limit", 0, "print at most `n` rows (500 unless set, every row with --csv; 0 prints every row)")
	return cmd
}

// rowLimit is how many rows sql keeps: the --limit given, else every row
// under --csv and defaultSQLLimit otherwise. 0 means every row.
func rowLimit(cmd *cobra.Command, limit int, csvOut bool) int {
	switch {
	case cmd.Flags().Changed("limit"):
		return limit
	case csvOut:
		return 0
	}
	return defaultSQLLimit
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
		return "", &runtimeError{err: &refusalError{text: "cannot read the query from stdin: " + osreason.Reason(got.err), cause: got.err}}
	}
	if strings.TrimSpace(string(got.query)) == "" {
		return "", errSQLNeedsQuery
	}
	return string(got.query), nil
}

// truncationNote is the warning that sql cut its rows at limit.
func truncationNote(limit int) string {
	return "showing the first " + humanize.Count(limit, "row", "rows") + "; the query returned more; pass --limit 0 to print every row"
}

// queryFailure is the error sql reports for a failed query: the ruled
// refusal copy for each store refusal, otherwise err unchanged.
func queryFailure(err error) error {
	failure := report.ClassifyQueryFailure(err)
	switch failure.Kind {
	case report.QueryFailureUnprintable:
		return fmt.Errorf("%w; cast it in the query, e.g. CAST(%s AS VARCHAR)", failure.Err, failure.Detail)
	case report.QueryFailureRejected:
		return &refusalError{text: "query failed: " + failure.Detail, cause: err}
	case report.QueryFailureEmpty:
		return errSQLNeedsQuery
	case report.QueryFailureReadOnly:
		return &refusalError{text: "quarry sql only reads the store; change the data in Quicken and run quarry sync", cause: err}
	case report.QueryFailureExternalAccess:
		return &refusalError{text: "quarry sql reads only quarry's store; other files, databases and extensions are turned off", cause: err}
	case report.QueryFailureInterrupted:
		return &refusalError{text: "query interrupted", cause: err}
	case report.QueryFailureOther:
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
