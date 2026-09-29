package cli

import (
	"github.com/spf13/cobra"
)

// newAccountsCommand builds the accounts subcommand: list the store's
// accounts with their balances, closed ones only when --all is given.
func newAccountsCommand(newReport ReportFactory) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List accounts with their current balances",
		Long: `List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts show "not imported": quarry does not
import investment transactions yet, so it cannot compute their balance.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv, err := newReport(cmd.Context(), cmd.Name())
			if err != nil {
				return &runtimeError{err: err}
			}

			list, err := srv.Accounts(cmd.Context(), all)
			if err != nil {
				return &runtimeError{err: err}
			}

			if _, err := cmd.OutOrStdout().Write([]byte(renderAccounts(list))); err != nil {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include closed accounts")
	return cmd
}
