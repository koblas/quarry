package cli

import (
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// newStatusCommand builds the status subcommand: read the store's own
// description and its findings tally and render them, as JSON when *jsonOut is set.
func newStatusCommand(newReport ReportFactory, loadConfig ConfigLoader, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which snapshot the store was built from and what it holds",
		Long: `Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, the checks sync ran when it built the store, and how
many findings are open.

status reads quarry's store, and the config file for the findings you ignored;
it never looks at Quicken. Run quarry sync to bring the store up to date.`,
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

			ignore, warnings := statusIgnore(loadConfig)
			printConfigWarnings(cmd, warnings)
			findings := statusFindings{counts: report.CountFindings(st, ignore), ignoreKnown: len(warnings) == 0}

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderStatusJSON(st, findings, warnings) },
				func() string { return renderStatus(st, findings, srv.Home(), time.Now()) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "", nil)
		},
	}
}

// statusIgnore returns findings.ignore and the warnings to print. Status never refuses over the
// config: when it cannot be read the list is nil and warnings holds the one line saying why.
func statusIgnore(loadConfig ConfigLoader) ([]string, []string) {
	cfg, err := loadConfig("status")
	if err != nil {
		return nil, []string{"cannot tell which findings you ignored: " + config.Problem(err) +
			"; findings you ignored are counted as open"}
	}
	return cfg.Ignore, nil
}
