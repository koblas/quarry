package cli

import (
	"github.com/koblas/quarry/internal/platform/humanize"
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

// errSearchNegativeLimit refuses a --limit below zero.
var errSearchNegativeLimit = UsageError{msg: "--limit must be 0 or more; 0 prints every transaction"}

// searchAmounts reads --min and --max as cmd was given them, or refuses them as a UsageError. A flag that was
// given empty is refused, not ignored.
func searchAmounts(cmd *cobra.Command, least, most *string) (report.SearchAmounts, error) {
	if !cmd.Flags().Changed("min") {
		least = nil
	}
	if !cmd.Flags().Changed("max") {
		most = nil
	}
	amounts, err := report.ParseSearchAmounts(least, most)
	if err != nil {
		return report.SearchAmounts{}, UsageError{msg: err.Error()}
	}
	return amounts, nil
}

// searchArgs refuses, as a UsageError and before anything is read, more than one text, then a negative
// *limit, then blank text, then text and --category that are not valid UTF-8.
func searchArgs(limit *int, category *string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return UsageError{msg: "search takes one text; quote it as one argument"}
		}
		if *limit < 0 {
			return errSearchNegativeLimit
		}
		if len(args) == 1 {
			if err := report.CheckSearchText(&args[0]); err != nil {
				return UsageError{msg: err.Error()}
			}
			if err := report.CheckUTF8(report.SearchInputText, &args[0]); err != nil {
				return UsageError{msg: err.Error()}
			}
		}
		if err := report.CheckUTF8(report.SearchInputCategory, category); err != nil {
			return UsageError{msg: err.Error()}
		}
		return nil
	}
}

// searchLimit is how many transactions search keeps: the --limit given, else defaultSearchLimit. 0 means every one.
func searchLimit(cmd *cobra.Command, limit int) int {
	if cmd.Flags().Changed("limit") {
		return limit
	}
	return defaultSearchLimit
}

// searchCutNote is the warning that s lists only its newest transactions of the matches.
func searchCutNote(s report.Search) string {
	return "showing the newest " + humanize.Thousands(len(s.Rows)) + " of " + humanize.Thousands(s.Matched) +
		" matching transactions; pass --limit 0 to list every one"
}

// newSearchCommand builds search: the newest --limit transactions containing the text, dated in the --since/--until
// period, in the --account accounts. It reads neither the clock nor the config file.
func newSearchCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
	var (
		flags                    reportFlags
		limit                    int
		minArg, maxArg, category string
	)
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
		Args: searchArgs(&limit, &category),
		RunE: func(cmd *cobra.Command, args []string) error {
			amounts, err := searchAmounts(cmd, &minArg, &maxArg)
			if err != nil {
				return err
			}
			window, err := flags.searchWindow(cmd)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			req := report.SearchRequest{Window: window, Accounts: flags.accounts, Amounts: amounts, Limit: searchLimit(cmd, limit)}
			if len(args) == 1 {
				req.Text = &args[0]
			}
			if cmd.Flags().Changed("category") {
				req.Category = &category
			}
			found, err := srv.Search(cmd.Context(), req)
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := document.SearchWarnings(found)
			if found.Truncated() {
				warnings = append(warnings, searchCutNote(found))
			}

			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) { return marshalDocument(document.NewSearch(found, warnings)) },
				func() string { return renderSearch(found) })
		},
	}
	flags.bind(cmd, searchFlagHelp)
	cmd.Flags().StringVar(&category, "category", "", "list only transactions with a split in this category or one under it, by full `path` such as Food:Groceries")
	cmd.Flags().StringVar(&minArg, "min", "", "list only transactions of at least this `amount`, sign ignored, in the account's own currency")
	cmd.Flags().StringVar(&maxArg, "max", "", "list only transactions of at most this `amount`, sign ignored, in the account's own currency")
	// The flag's own default is 0 so help prints no "(default ...)" beside the ruled text; searchLimit applies 500.
	cmd.Flags().IntVar(&limit, "limit", 0, "print at most `n` transactions, newest first (500 unless set; 0 prints every one)")
	return cmd
}
