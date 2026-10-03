package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// defaultSearchLimit is how many transactions search lists.
const defaultSearchLimit = 500

// searchFlagHelp is the usage text of search's --since, --until and --account flags.
var searchFlagHelp = reportFlagHelp{
	since:   "list transactions dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default the first transaction)",
	until:   "list transactions dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default no end, future-dated included)",
	account: "search only the account with this `name` or id; repeat for more",
}

// newSearchCommand builds search: the newest transactions dated in the --since/--until period, in the --account accounts.
// It reads neither the clock nor the config file.
func newSearchCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
	var flags reportFlags
	cmd := &cobra.Command{
		Use:   "search [text]",
		Short: "Find transactions by payee, memo, amount, date, account or category",
		Long: `Find transactions by text, date, account, category or amount, newest
first. The text matches payee names, transaction memos and split memos,
ignoring letter case; every character is literal, so % and _ match only
themselves. Leave the text out to search by the flags alone, or pass
nothing at all to list the newest transactions. Text that starts with -
goes after --: quarry search -- "-50% off"

Every transaction is searched, closed accounts included. Transfers
between your own accounts and transactions Quicken's reports leave out
are listed too, flagged transfer or excluded, because quarry spend and
quarry cashflow do not count them. Excluded means the transaction is
marked "exclude from reports" in Quicken, or its account is not used in
reports or uses linked account tracking. Spend also leaves out Quicken's
system categories; those are not flagged.

Amounts are in each account's own currency and are never converted.
--min and --max compare the amount without its sign, so --min 100 finds
charges and deposits of 100.00 or more; give both the same value to find
one amount. --category matches a split in that category or in any
category under it, by full path in any letter case.

Without --since and --until every date is searched, future-dated
transactions included. At most --limit transactions are printed (500
unless set); when more match, quarry says so on stderr.`,
		Example: `  quarry search costco
  quarry search --min 42.17 --max 42.17
  quarry search "e-transfer" --account Chequing --since 2026-01
  quarry search --category Food --since 2026-09 --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			window, err := flags.searchWindow(cmd)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			found, err := srv.Search(cmd.Context(), report.SearchRequest{Window: window, Accounts: flags.accounts, Limit: defaultSearchLimit})
			if err != nil {
				return &runtimeError{err: err}
			}

			return emitReport(cmd, *jsonOut, nil,
				func() ([]byte, error) { return marshalDocument(document.NewSearch(found, nil)) },
				func() string { return renderSearch(found) })
		},
	}
	flags.bind(cmd, searchFlagHelp)
	return cmd
}
