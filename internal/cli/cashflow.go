package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newCashFlowCommand builds cashflow: the income, spending and savings rate in the --since/--until period per month or year.
func newCashFlowCommand(newReport ReportFactory, now func() time.Time, jsonOut *bool) *cobra.Command {
	var by string
	var accounts []string
	var period windowFlags
	cmd := &cobra.Command{
		Use:   "cashflow",
		Short: "Show income, spending and savings rate by month or year",
		Long: `Show income, spending and what was left over for each month or year, in
each account's own currency: CAD and USD are listed separately, never added
together.

Income and spending follow the same rules as quarry spend: transfers between
your own accounts, Quicken's system categories and transactions marked
"exclude from reports" are left out, and refunds are netted. Accounts Quicken
leaves out of reports are left out here too; quarry accounts marks them "not
in reports". Uncategorized splits count as income when they bring money in
and as spending when they take money out. The Spent column equals quarry
spend's total for the same period and accounts.

Savings rate is net divided by income, and shows n/a when income is zero or
less. A period that --since or --until cuts short is marked partial.`,
		Example: `  quarry cashflow
  quarry cashflow --by year --since 2020 --until 2025
  quarry cashflow --account Chequing --json`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmd.Flags().StringVar(&by, "by", "month", "group by `period`: month or year")
	period.bind(cmd)
	cmd.Flags().StringArrayVar(&accounts, "account", nil, "count only the account with this `name` or id; repeat for more")
	return cmd
}
