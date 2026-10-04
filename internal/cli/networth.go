package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newNetWorthCommand builds networth: the balance of each account type and currency on one day.
func newNetWorthCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:  "networth",
		Args: currency.args,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}
