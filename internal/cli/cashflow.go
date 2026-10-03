package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// cashFlowCommand is the word that names cashflow on the command line and in its warnings.
const cashFlowCommand = "cashflow"

// cashFlowPeriod is how cashflow presents one --by value: the column-1 header.
type cashFlowPeriod struct {
	header string
}

// cashFlowPeriods holds every --by value cashflow reads, indexed by period.
var cashFlowPeriods = [...]cashFlowPeriod{
	store.CashFlowByMonth: {header: "Month"},
	store.CashFlowByYear:  {header: "Year"},
}

// errCashFlowByUnknown refuses a --by that names no period cashflow reads.
var errCashFlowByUnknown = UsageError{msg: "--by must be month or year"}

// parseCashFlowPeriod returns the period named by a --by value, or errCashFlowByUnknown.
func parseCashFlowPeriod(name string) (store.CashFlowPeriod, error) {
	period, ok := store.ParseCashFlowPeriod(name)
	if !ok {
		return 0, errCashFlowByUnknown
	}
	return period, nil
}

// newCashFlowCommand builds cashflow: the income, spending and savings rate in the --since/--until period per month or year.
func newCashFlowCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var by string
	var flags reportFlags
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:   cashFlowCommand,
		Short: "Show income, spending and savings rate by month or year",
		Long: `Show income, spending and what was left over for each month or year.

` + reportCurrencyLong + `

Income and spending follow the same rules as quarry spend: transfers between
your own accounts, Quicken's system categories and transactions marked
"exclude from reports" are left out, and refunds are netted. Accounts Quicken
leaves out of reports ("not in reports" in quarry accounts) and accounts
that use Quicken's linked account tracking ("linked tracking") are left out
here too. Uncategorized splits count as income when they bring money in and
as spending when they take money out. The Spent column equals quarry spend's
total for the same period, accounts and currency.

Savings rate is net divided by income, and shows n/a when income is zero or
less. A period that --since or --until cuts short is marked partial.`,
		Example: `  quarry cashflow
  quarry cashflow --by year --since 2020 --until 2025
  quarry cashflow --account Chequing --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			period, err := parseCashFlowPeriod(by)
			if err != nil {
				return err
			}

			resolved, err := flags.window(cmd, now())
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

			flow, err := srv.CashFlow(cmd.Context(), report.CashFlowRequest{Window: resolved, By: period, Accounts: flags.accounts, Currency: reportCurrency})
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := document.CashFlowWarnings(flow, cashFlowCommand)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) { return renderCashFlowJSON(flow, withConfigWarnings(configWarnings, warnings)) },
				func() string { return renderCashFlow(flow) })
		},
	}
	cmd.Flags().StringVar(&by, "by", store.CashFlowByMonth.String(), "group by `period`: month or year")
	flags.bind(cmd, transactionFlagHelp)
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}
