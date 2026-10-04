package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// newHoldingsCommand builds holdings: the securities held in each account today, with their value.
func newHoldingsCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
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

The total is the value of the securities only. Cash held in investment
accounts is not included, so it is not those accounts' balance.`,
		Example: `  quarry holdings
  quarry holdings --as-of 2025-12-31
  quarry holdings --account RRSP --currency native --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reportCurrency, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			holdings, err := srv.Holdings(cmd.Context(), report.HoldingsRequest{AsOf: report.Today(now()), Currency: reportCurrency})
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
	currency.bind(cmd, holdingsCurrencyHelp)
	return cmd
}
