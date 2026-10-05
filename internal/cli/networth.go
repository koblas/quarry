package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// newNetWorthCommand builds networth: the balance of each account type and currency on one day.
func newNetWorthCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
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

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			netWorth, err := srv.NetWorth(cmd.Context(), report.NetWorthRequest{AsOf: report.Today(now()), Currency: reportCurrency})
			if err != nil {
				return &runtimeError{err: err}
			}

			return emitReport(cmd, *jsonOut, nil,
				func() ([]byte, error) { return renderNetWorthJSON(netWorth, withConfigWarnings(configWarnings, nil)) },
				func() string { return renderNetWorth(netWorth) })
		},
	}
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}
