package cli

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// defaultSQLLimit is how many rows sql prints unless --limit says otherwise.
const defaultSQLLimit = 500

// newSQLCommand builds sql: one query run verbatim against the store, its
// result printed as a table of at most --limit rows.
func newSQLCommand(newReport ReportFactory) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "sql <query>",
		Short: "Run a read-only SQL query against quarry's store",
		Long: `Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions.

Pass the query as one quoted argument, or - to read it from stdin. Amounts
are DECIMAL(18,2) in each account's own currency; negative is money leaving
the account. Transfers between your own accounts are in the transfers table
and splits.transfer_account_id, never in a category kind. List the tables
and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set); when there are more,
quarry says so on stderr. --limit 0 prints every row.`,
		Example: `  quarry sql "SELECT name, currency FROM accounts WHERE NOT closed"
  quarry sql --limit 0 --json - < monthly.sql`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := newReport(cmd.Context(), cmd.Name())
			if err != nil {
				return &runtimeError{err: err}
			}

			result, err := srv.Query(cmd.Context(), args[0], limit)
			if err != nil {
				return &runtimeError{err: queryFailure(err)}
			}

			if _, err := cmd.OutOrStdout().Write([]byte(renderSQLTable(result.QueryResult))); err != nil {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", defaultSQLLimit, "print at most `n` rows (0 prints every row)")
	return cmd
}

// queryFailure is the error sql reports for a failed query: the refusal
// copy for a value quarry cannot print, otherwise err unchanged.
func queryFailure(err error) error {
	if unprintable, ok := errors.AsType[*store.UnprintableValueError](err); ok {
		return fmt.Errorf("%w; cast it in the query, e.g. CAST(%s AS VARCHAR)", unprintable, unprintable.Column)
	}
	return err
}
