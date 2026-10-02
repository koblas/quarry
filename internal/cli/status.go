package cli

import (
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
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

			ignore, warnings, warningsAbsolute := statusIgnore(loadConfig)
			printConfigWarnings(cmd, warnings)
			findings := statusFindings{counts: report.CountFindings(st, ignore), ignoreKnown: len(warnings) == 0}

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

// statusIgnore returns findings.ignore and the warnings to print, ~-abbreviated and then with absolute
// paths for --json. Status never refuses over the config: when it cannot be read the list is nil and
// each warnings list holds the one line saying why.
func statusIgnore(loadConfig ConfigLoader) ([]string, []string, []string) {
	cfg, err := loadConfig("status")
	if err != nil {
		return nil, []string{cannotTellIgnored(config.Problem(err))}, []string{cannotTellIgnored(config.ProblemAbsolute(err))}
	}
	return cfg.Ignore, nil, nil
}

// cannotTellIgnored is the warning for a config that cannot be read, naming problem.
func cannotTellIgnored(problem string) string {
	return "cannot tell which findings you ignored: " + problem + "; findings you ignored are counted as open"
}
