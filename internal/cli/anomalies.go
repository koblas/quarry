package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// anomaliesCommand is the word that names anomalies on the command line and in its warnings.
const anomaliesCommand = "anomalies"

// anomaliesFlagHelp is the usage text of anomalies' --since, --until and --account flags.
var anomaliesFlagHelp = reportFlagHelp{
	since:   "list charges dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)",
	until:   "list charges dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)",
	account: "list only charges in the account with this `name` or id; repeat for more",
}

// newAnomaliesCommand builds anomalies: the charges in the --since/--until period unusually large for their payee or category.
func newAnomaliesCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	flags := reportFlags{chargesCommand: anomaliesCommand}
	var currency currencyFlag
	cmd := &cobra.Command{
		Use:   anomaliesCommand,
		Short: "List charges unusually large for their payee or category",
		Long: `List charges that are unusually large: more than 2 times the median of the
payee's earlier charges, when there are at least 3, or else more than 5
times the median of the category's earlier charges, when there are at least
10. Charges under 100.00 are never listed. Charges follow the rules of
quarry spend, and a transaction counts once, with all its splits; an
uncategorized or split charge from a payee with little history cannot be
judged. Possible duplicates are listed by quarry findings, not here. Charges
dated after today are left out, even with a later --until.

--since and --until choose which charges to list; each is compared with
every earlier charge, however old. --account lists only charges in those
accounts; the payee's charges in other accounts still count as history.`,
		Example: `  quarry anomalies
  quarry anomalies --since 2026-09 --until 2026-09
  quarry anomalies --account "Visa Infinite" --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			at := now()
			resolved, err := flags.window(cmd, at)
			if err != nil {
				return err
			}

			_, configWarnings, err := currency.resolve(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			found, err := srv.Anomalies(cmd.Context(), report.AnomaliesRequest{Window: resolved, Now: at, Accounts: flags.accounts})
			if err != nil {
				return &runtimeError{err: err}
			}

			warnings := anomaliesWarnings(found)
			return emitReport(cmd, *jsonOut, warnings,
				func() ([]byte, error) {
					return renderAnomaliesJSON(found, withConfigWarnings(configWarnings, warnings))
				},
				func() string { return renderAnomalies(found) })
		},
	}
	flags.bind(cmd, anomaliesFlagHelp)
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}

// anomaliesWarnings is a's warnings, unprefixed and never nil: one per named account left out of the
// report, then a note when no charge was checked in the period.
func anomaliesWarnings(a report.Anomalies) []string {
	warnings := leftOutWarnings(a.Accounts, anomaliesCommand)
	if a.Checked == 0 {
		warnings = appendEmptyWindowWarning(warnings, "unusually large charges", a.Accounts, a.Window, a.Transactions)
	}
	return warnings
}
