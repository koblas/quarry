package cli

import "github.com/spf13/cobra"

// newAccountsCommand builds accounts: balances per account, closed ones only
// with --all, as JSON when *jsonOut is set.
func newAccountsCommand(newReport ReportFactory, loadConfig ConfigLoader, jsonOut *bool) *cobra.Command {
	var all bool
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List accounts with their current balances",
		Long: `List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts show "not imported": quarry does not
import investment transactions yet, so it cannot compute their balance.`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reporting, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			listing, err := srv.Accounts(cmd.Context(), all, reporting)
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := []string{}
			if listing.AllHidden() {
				warnings = append(warnings, allClosedNote(listing.Hidden))
			}

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) {
					return renderAccountsJSON(listing.AccountList, withConfigWarnings(configWarnings, warnings))
				},
				func() string { return renderAccounts(listing.AccountList) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "quarry: warning: ", warnings)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include closed accounts")
	currency.bind(cmd, accountsCurrencyHelp)
	return cmd
}
