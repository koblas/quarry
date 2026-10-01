package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// cashFlowCommand is the word that names cashflow on the command line and in its warnings.
const cashFlowCommand = "cashflow"

// cashFlowPeriod is how cashflow presents one --by value: the flag word and the column-1 header.
type cashFlowPeriod struct {
	name, header string
}

// cashFlowPeriods holds every --by value cashflow reads, indexed by period.
var cashFlowPeriods = [...]cashFlowPeriod{
	store.CashFlowByMonth: {name: "month", header: "Month"},
	store.CashFlowByYear:  {name: "year", header: "Year"},
}

// errCashFlowByUnknown refuses a --by that names no period cashflow reads.
var errCashFlowByUnknown = UsageError{msg: "--by must be month or year"}

// parseCashFlowPeriod returns the period named by a --by value, or errCashFlowByUnknown.
func parseCashFlowPeriod(name string) (store.CashFlowPeriod, error) {
	for period, p := range cashFlowPeriods {
		if p.name == name {
			return store.CashFlowPeriod(period), nil
		}
	}
	return 0, errCashFlowByUnknown
}

// newCashFlowCommand builds cashflow: the income, spending and savings rate in the --since/--until period per month or year.
func newCashFlowCommand(newReport ReportFactory, now func() time.Time, jsonOut *bool) *cobra.Command {
	var by string
	var flags reportFlags
	cmd := &cobra.Command{
		Use:   cashFlowCommand,
		Short: "Show income, spending and savings rate by month or year",
		Long: `Show income, spending and what was left over for each month or year, in
each account's own currency: CAD and USD are listed separately, never added
together.

Income and spending follow the same rules as quarry spend: transfers between
your own accounts, Quicken's system categories and transactions marked
"exclude from reports" are left out, and refunds are netted. Accounts Quicken
leaves out of reports ("not in reports" in quarry accounts) and accounts
that use Quicken's linked account tracking ("linked tracking") are left out
here too. Uncategorized splits count as income when they bring money in and
as spending when they take money out. The Spent column equals quarry spend's
total for the same period and accounts.

Savings rate is net divided by income, and shows n/a when income is zero or
less. A period that --since or --until cuts short is marked partial.`,
		Example: `  quarry cashflow
  quarry cashflow --by year --since 2020 --until 2025
  quarry cashflow --account Chequing --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			period, err := parseCashFlowPeriod(by)
			if err != nil {
				return err
			}

			resolved, err := flags.window(cmd, now())
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			flow, err := srv.CashFlow(cmd.Context(), report.CashFlowRequest{Window: resolved, By: period, Accounts: flags.accounts})
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := cashFlowWarnings(flow)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) { return renderCashFlowJSON(flow, warnings) },
				func() string { return renderCashFlow(flow) })
		},
	}
	cmd.Flags().StringVar(&by, "by", cashFlowPeriods[store.CashFlowByMonth].name, "group by `period`: month or year")
	flags.bind(cmd, transactionFlagHelp)
	return cmd
}

// cashFlowWarnings is c's warnings, unprefixed and never nil: one per named account left out (W2 or W3),
// then a note that the window held no income or spending.
func cashFlowWarnings(c report.CashFlow) []string {
	warnings := leftOutWarnings(c.Accounts, cashFlowCommand)
	if c.Empty() {
		warnings = appendEmptyWindowWarning(warnings, "income or spending", c.Accounts, c.Window, c.Transactions)
	}
	return warnings
}
