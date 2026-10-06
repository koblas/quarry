package cli

import (
	"slices"
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
			at := now()
			var given *string
			named := cmd.Flags().Changed(summaryMonthFlagName)
			if named {
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
			if choices.cannotTell != "" {
				printConfigWarnings(cmd, []string{choices.cannotTell})
			}
			own := document.SummaryWarnings(summary, summaryAgain(resolved, named), document.NativeFlag)
			return emitReport(cmd, *jsonOut, own,
				func() ([]byte, error) {
					return renderSummaryJSON(summary, findings, withConfigWarnings(choices.configWarningsAbsolute(), own))
				},
				func() string { return renderSummary(summary, findings, at) })
		},
	}
	cmd.Flags().StringVar(&month, summaryMonthFlagName, "", "summarize month `YYYY-MM` instead of last month; it must have ended")
	currency.bind(cmd, reportCurrencyHelp)
	return cmd
}

// summaryAgain is the phrase a warning ends with to repeat this summary: the command as typed, with --month
// only when the month was named.
func summaryAgain(month report.Month, named bool) string {
	if named {
		return "run quarry summary --month " + month.String() + " again"
	}
	return "run quarry summary again"
}

// summaryChoice is what the config decides for a summary; cannotTell, printed after the report is read, says why
// ignoreKnown is false. The Absolute fields name the config by its absolute path, for --json.
type summaryChoice struct {
	currency           money.Currency
	ignore             []string
	classification     report.Classification
	ignoreKnown        bool
	cannotTell         string
	cannotTellAbsolute string
	warningsAbsolute   []string
}

// configWarningsAbsolute is the config's --json warnings: its own, then the cannot-tell one when the file
// could not be read.
func (c summaryChoice) configWarningsAbsolute() []string {
	warnings := slices.Clone(c.warningsAbsolute)
	if c.cannotTellAbsolute != "" {
		warnings = append(warnings, c.cannotTellAbsolute)
	}
	return warnings
}

// summaryChoices loads the config once and prints its warnings. An unreadable config is a runtimeError, or
// with --currency findings counted without the choices and the warning left in cannotTell.
func summaryChoices(cmd *cobra.Command, loadConfig ConfigLoader, currency currencyFlag) (summaryChoice, error) {
	cfg, err := loadConfig(cmd.Name())
	if err != nil {
		if !cmd.Flags().Changed(currencyFlagName) {
			return summaryChoice{}, &runtimeError{err: err}
		}
		return summaryChoice{
			currency:           currency.in(cmd, config.Config{}),
			cannotTell:         document.CannotTellChoices(config.Problem(err)),
			cannotTellAbsolute: document.CannotTellChoices(config.ProblemAbsolute(err)),
		}, nil
	}
	printConfigWarnings(cmd, cfg.Warnings)
	return summaryChoice{
		currency:         currency.in(cmd, cfg),
		ignore:           cfg.Ignore,
		classification:   classificationOf(cfg),
		ignoreKnown:      true,
		warningsAbsolute: cfg.WarningsAbsolute,
	}, nil
}
