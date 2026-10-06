package cli

import (
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// summaryCommand is the word that names summary on the command line and in its warnings.
const summaryCommand = "summary"

// summaryMonthFlagName is the name of summary's --month flag.
const summaryMonthFlagName = "month"

// summaryJSONRefusal is what --json gets until summary has a document to print.
const summaryJSONRefusal = "summary --json is not available yet"

// newSummaryCommand builds summary: one month's unusually large charges, new recurring charges, net worth and findings.
func newSummaryCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
	var month string
	cmd := &cobra.Command{
		Use:   summaryCommand,
		Short: "Summarize a month: unusual charges, new recurring charges, net worth and findings",
		//nolint:dupword // the ruled copy ends the example with "summary" and starts the next paragraph with it
		Long: `Summarize one month, last month unless --month names another: how fresh
the store is and how many findings are open, the month's unusually large
charges, the recurring charges new in the month, and net worth at the end
of the month beside the end of the month before. It is meant to run once
a month after quarry sync, for example from launchd:

  quarry sync; quarry summary

summary only reads quarry's store; it never runs sync and never looks at
Quicken, so it works with Quicken closed. When the store was built from a
snapshot taken before the month ended, summary warns, since transactions
from the rest of the month are missing; open your Quicken file, run quarry
sync, then run summary again.

Unusually large charges are those quarry anomalies lists for the month,
for example quarry anomalies --since 2026-09 --until 2026-09.

A recurring charge is new in the first month quarry recurring can list
it: usually the month of its third monthly or quarterly charge, fourth
weekly charge or second yearly charge, later when its amount changed too
often before then. A recurring charge that starts again after a charge
off schedule is new only if it had ended first, with no charge for 14
days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days
(yearly). Recurring charges are judged as of the month's last day, so
later charges never change a past month's summary.

Net worth is what quarry networth lists at the two month ends, for
example quarry networth --since 2026-08 --until 2026-09; Change is the
difference. In CAD or USD it includes changes in the exchange rate.

Findings counts the findings open in the store; new and fixed are what
the last sync found, whenever it ran.

Months begin and end at midnight in this Mac's time zone. Amounts are in
CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
With --currency native, CAD and USD are listed separately, never added
together.`,
		Example: `  quarry summary
  quarry summary --month 2026-08 --currency USD
  quarry summary --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *jsonOut {
				return UsageError{msg: summaryJSONRefusal}
			}

			at := now()
			var given *string
			if cmd.Flags().Changed(summaryMonthFlagName) {
				given = &month
			}
			resolved, err := report.ParseMonth(given, at)
			if err != nil {
				return UsageError{msg: err.Error()}
			}

			choices, err := summaryChoices(cmd, loadConfig, currency)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			summary, err := srv.Summary(cmd.Context(), report.SummaryRequest{Month: resolved, Currency: choices.currency})
			if err != nil {
				return &runtimeError{err: err}
			}

			findings := document.FindingsTally{
				Counts:      report.CountFindings(summary.Status, choices.ignore, choices.classification),
				IgnoreKnown: choices.ignoreKnown,
			}
			return writeResult(cmd, []byte(renderSummary(summary, findings, at)))
		},
	}
	cmd.Flags().StringVar(&month, summaryMonthFlagName, "", "summarize month `YYYY-MM` instead of last month; it must have ended")
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}

// summaryChoice is what the config file decides for a summary: its currency and the findings choices. ignoreKnown
// is false when the file could not be read, so no ignored count can be said.
type summaryChoice struct {
	currency       money.Currency
	ignore         []string
	classification report.Classification
	ignoreKnown    bool
}

// summaryChoices loads the config once and prints its warnings. An unreadable config is a runtimeError, or
// with --currency one warning and findings counted without the choices.
func summaryChoices(cmd *cobra.Command, loadConfig ConfigLoader, currency currencyFlag) (summaryChoice, error) {
	cfg, err := loadConfig(cmd.Name())
	if err != nil {
		if !cmd.Flags().Changed(currencyFlagName) {
			return summaryChoice{}, &runtimeError{err: err}
		}
		printConfigWarnings(cmd, []string{document.CannotTellChoices(config.Problem(err))})
		return summaryChoice{currency: currency.in(cmd, config.Config{})}, nil
	}
	printConfigWarnings(cmd, cfg.Warnings)
	return summaryChoice{
		currency:       currency.in(cmd, cfg),
		ignore:         cfg.Ignore,
		classification: classificationOf(cfg),
		ignoreKnown:    true,
	}, nil
}
