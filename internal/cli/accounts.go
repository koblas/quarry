package cli

import (
	"slices"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// newAccountsCommand builds accounts: balances per account, closed ones only
// with --all, as JSON when *jsonOut is set. It always reads the config, for the
// account classification, even when --currency is given.
func newAccountsCommand(newReport ReportFactory, loadConfig ConfigLoader, jsonOut *bool) *cobra.Command {
	var all bool
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List accounts with their current balances",
		Long: `List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts' balance is the cash in them plus the
value of their holdings today, each at the latest price Quicken recorded
(quarry holdings lists them).

A column shows each balance in the reporting currency (--currency, else
reporting.currency in the config file, else CAD) at today's Bank of
Canada rate, or the latest earlier one; --currency native leaves it
out. quarry does not add balances together here; quarry networth does.

Status says registered for an account listed in accounts.registered in
the config file, and unclassified for a brokerage or retirement account
in neither accounts.registered nor accounts.non-registered; quarry
findings lists those.`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := readConfig(cmd, loadConfig)
			if err != nil {
				return err
			}
			reporting := currency.in(cmd, cfg)

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			listing, err := srv.Accounts(cmd.Context(), all, reporting, classificationOf(cfg))
			if err != nil {
				return &runtimeError{err: err}
			}

			printConfigWarnings(cmd, document.UnmatchedAccountWarnings(homepath.Abbreviate(srv.Home(), cfg.Path), listing.UnmatchedAccounts))

			warnings := []string{}
			if listing.AllHidden() {
				warnings = append(warnings, allClosedNote(listing.Hidden))
			}
			warnings = append(warnings, document.AccountsWarnings(listing)...)
			warnings = append(warnings, accountsFXWarnings(listing)...)

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) {
					return renderAccountsJSON(listing, withConfigWarnings(slices.Concat(cfg.WarningsAbsolute, document.UnmatchedAccountWarnings(cfg.Path, listing.UnmatchedAccounts)), warnings))
				},
				func() string { return renderAccounts(listing) })
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
