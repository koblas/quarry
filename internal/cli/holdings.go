package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

const holdingsAccountFlagHelp = "list only the account with this `name` or id; repeat for more"

const holdingsAsOfFlagHelp = "value holdings on `date` (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)"

// newHoldingsCommand builds holdings: the securities held in each account on one day, with their value.
func newHoldingsCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
	var asOfFlag string
	var accounts []string
	cmd := &cobra.Command{
		Use:   "holdings",
		Short: "List the securities held in each account and their value",
		Long: `List each security held in a brokerage or retirement account on one day
(--as-of, default today): its share count, the latest price Quicken
recorded on or before that day with that price's date, and its value,
shares times price rounded to the cent. Share counts are the ones quarry
sync checks against Quicken. A holding with no price on or before that day
is listed with no value and left out of the total.

Values are in each security's own currency. A column shows each value in
the reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the Bank of Canada rate for the --as-of day, or the
latest earlier one; --currency native leaves it out and totals each
currency separately.

The total is the value of the securities only, without the cash held in
investment accounts; quarry accounts shows each account's balance, cash
included.`,
		Example: `  quarry holdings
  quarry holdings --as-of 2025-12-31
  quarry holdings --account RRSP --currency native --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var given *string
			if cmd.Flags().Changed("as-of") {
				given = &asOfFlag
			}
			asOf, err := report.ResolveAsOf(given, now())
			if err != nil {
				return UsageError{msg: err.Error()}
			}

			reportCurrency, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			holdings, err := srv.Holdings(cmd.Context(), report.HoldingsRequest{AsOf: asOf, Currency: reportCurrency, Accounts: accounts})
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := document.HoldingsWarnings(holdings)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) {
					return renderHoldingsJSON(holdings, withConfigWarnings(configWarnings, warnings))
				},
				func() string { return renderHoldings(holdings) })
		},
	}
	cmd.Flags().StringVar(&asOfFlag, "as-of", "", holdingsAsOfFlagHelp)
	cmd.Flags().StringArrayVar(&accounts, "account", nil, holdingsAccountFlagHelp)
	currency.bind(cmd, holdingsCurrencyHelp)
	return cmd
}
