package cli

import (
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// spendCommand is the word that names spend on the command line and in its warnings.
const spendCommand = "spend"

// newSpendCommand builds spend: the spending in the --since/--until period (default this year to now()) grouped by --by.
func newSpendCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var by string
	var flags reportFlags
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:   spendCommand,
		Short: "Show spending by category, payee, tag or month",
		Long: `Show how much you spent, grouped by category, payee, tag or month.

` + reportCurrencyLong + `

Spending is every split in an expense category, plus uncategorized splits
that take money out. Refunds in an expense category are netted against it,
so a category can come out negative. Transfers between your own accounts,
splits in Quicken's system categories and transactions marked "exclude from
reports" in Quicken are left out. So are accounts Quicken leaves out of
reports (quarry accounts marks them "not in reports") and accounts that use
Quicken's linked account tracking (marked "linked tracking"). Closed
accounts are included.

The period runs from --since to --until, both included; a bare year or month
covers all of it (--since 2024 --until 2024 is the whole of 2024). Without
them it is this year up to today, so future-dated transactions are left out
unless --until is later than today.

A split with more than one tag counts under each of them, so with --by tag
the rows can add up to more than the total.`,
		Example: `  quarry spend
  quarry spend --by payee --since 2025-01 --until 2025-03
  quarry spend --since 2024 --until 2024 --json
  quarry spend --account "Visa Infinite" --account Chequing`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			group, err := parseSpendGrouping(by)
			if err != nil {
				return err
			}

			window, err := flags.window(cmd, now())
			if err != nil {
				return err
			}

			reportCurrency, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			spending, err := srv.Spend(cmd.Context(), report.SpendRequest{Window: window, By: group, Accounts: flags.accounts, Currency: reportCurrency})
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := spendWarnings(spending)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) {
					return renderSpendingJSON(spending, withConfigWarnings(configWarnings, warnings))
				},
				func() string { return renderSpending(spending) })
		},
	}
	cmd.Flags().StringVar(&by, "by", spendGroupings[store.SpendByCategory].name, "group spending by `group`: category, payee, tag or month")
	flags.bind(cmd, transactionFlagHelp)
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}

// spendWarnings is s's warnings, unprefixed and never nil: one per named account left out,
// then the unconverted-amounts note, the multi-tag-splits note, and a note that the window held no spending.
func spendWarnings(s report.Spending) []string {
	warnings := append(leftOutWarnings(s.Accounts, spendCommand), unconvertedWarnings(s.Currency, s.Unconverted)...)
	if s.By == store.SpendByTag && s.MultiTagSplits > 0 {
		warnings = append(warnings, humanize.Count(s.MultiTagSplits, "split carries", "splits carry")+
			" more than one tag, so the rows add up to more than the total")
	}
	if s.Empty() {
		warnings = appendEmptyWindowWarning(warnings, "spending", s.Accounts, s.Window, s.Transactions)
	}
	return warnings
}
