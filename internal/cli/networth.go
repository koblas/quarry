package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// netWorthAsOfConflict is the refusal of --as-of beside --since or --until.
const netWorthAsOfConflict = "--as-of cannot be combined with --since or --until; pass --as-of for one day, or --since and --until for month ends"

// newNetWorthCommand builds networth: the balance of each account type and currency on one day.
func newNetWorthCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
	var asOfFlag, since, until string
	cmd := &cobra.Command{
		Use:   "networth",
		Short: "Show net worth today or at each month end, by account type and currency",
		Long: `Show net worth on one day (--as-of, default today), or at the end of each
month from --since to --until: account balances added up by account type
and currency. A balance is the sum of the account's transactions dated
that day or earlier; a brokerage or retirement account adds the value of
its holdings that day, each at the latest price Quicken recorded on or
before it (quarry holdings lists them). Credit card, loan and other
liability balances are negative, so they reduce the total. Closed
accounts count with their balance on the day. Accounts Quicken leaves out
of reports ("not in reports" or "linked tracking" in quarry accounts) are
left out, as Quicken's reports do.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each account's balance converts at the Bank of Canada rate for the day it
is valued on, or the latest earlier rate, and is rounded to the cent
before it is added. With --currency native, CAD and USD are listed
separately, never added together.

Month ends after today are not listed; a history that reaches this month
ends with today.`,
		Example: `  quarry networth
  quarry networth --as-of 2025-12-31
  quarry networth --since 2020 --currency native --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reportCurrency, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			historyGiven := cmd.Flags().Changed("since") || cmd.Flags().Changed("until")
			var asOfGiven *string
			if cmd.Flags().Changed("as-of") {
				if historyGiven {
					return UsageError{msg: netWorthAsOfConflict}
				}
				asOfGiven = &asOfFlag
			}
			asOf, err := report.ResolveAsOf(asOfGiven, report.NetWorthNoun, now())
			if err != nil {
				return UsageError{msg: err.Error()}
			}

			request := report.NetWorthRequest{AsOf: asOf, Currency: reportCurrency}
			if historyGiven {
				sincePtr, untilPtr := changedBounds(cmd, since, until)
				window, err := report.ParseMonthEndWindow(sincePtr, untilPtr, now())
				if err != nil {
					return UsageError{msg: err.Error()}
				}
				request.Window = &window
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			netWorth, err := srv.NetWorth(cmd.Context(), request)
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := document.NetWorthWarnings(netWorth, document.NativeFlag)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) {
					return renderNetWorthJSON(netWorth, withConfigWarnings(configWarnings, warnings))
				},
				func() string { return renderNetWorth(netWorth) })
		},
	}
	cmd.Flags().StringVar(&asOfFlag, "as-of", "", "value net worth on `date` (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)")
	cmd.Flags().StringVar(&since, "since", "", "list net worth at each month end on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year when --until is given)")
	cmd.Flags().StringVar(&until, "until", "", "list net worth at each month end on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today; a later date means today)")
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}
