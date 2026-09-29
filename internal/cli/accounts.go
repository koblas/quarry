package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newAccountsCommand builds accounts: balances per account, closed ones only
// with --all, as JSON when *jsonOut is set.
func newAccountsCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
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

			listing, err := srv.Accounts(cmd.Context(), all)
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := []string{}
			if listing.AllHidden() {
				warnings = append(warnings, allClosedNote(listing.Hidden))
			}

			var out []byte
			if *jsonOut {
				if out, err = renderAccountsJSON(listing.AccountList, warnings); err != nil {
					// unreachable: renderAccountsJSON's own error path is unreachable for any AccountList; see marshalDocument.
					return &runtimeError{err: err}
				}
			} else {
				out = []byte(renderAccounts(listing.AccountList))
			}

			if _, err := cmd.OutOrStdout().Write(out); err != nil {
				return &runtimeError{err: err}
			}
			for _, warning := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "quarry: "+warning)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include closed accounts")
	return cmd
}
