package cli

import (
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// newStatusCommand builds the status subcommand: read the store's own
// description and its findings tally and render them, as JSON when *jsonOut is set.
func newStatusCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which snapshot the store was built from and what it holds",
		Long: `Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, the checks sync ran when it built the store, and how
many findings are open.

status reads quarry's store, and the config file for the findings you ignored;
it never looks at Quicken. Run quarry sync to bring the store up to date.

Rates shows the span of Bank of Canada USD/CAD rates the store holds and,
when the last sync could not fetch new ones, why.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			st, err := srv.Status(cmd.Context())
			if err != nil {
				return &runtimeError{err: err}
			}

			ignore, classification, warnings, warningsAbsolute := statusChoices(loadConfig)
			printConfigWarnings(cmd, warnings)
			findings := document.FindingsTally{Counts: report.CountFindings(st, ignore, classification), IgnoreKnown: len(warnings) == 0}

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderStatusJSON(st, findings, warningsAbsolute) },
				func() string { return renderStatus(st, findings, srv.Home(), now()) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "", nil)
		},
	}
}

// statusChoices returns findings.ignore, the account classification, and the warnings (~-abbreviated, then
// absolute for --json). It never refuses: an unreadable config gives empty choices and one warning saying why.
func statusChoices(loadConfig ConfigLoader) ([]string, report.Classification, []string, []string) {
	cfg, err := loadConfig("status")
	if err != nil {
		return nil, report.Classification{}, []string{document.CannotTellChoices(config.Problem(err))}, []string{document.CannotTellChoices(config.ProblemAbsolute(err))}
	}
	return cfg.Ignore, classificationOf(cfg), nil, nil
}
