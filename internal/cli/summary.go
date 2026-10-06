package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// summaryCommand is the word that names summary on the command line and in its warnings.
const summaryCommand = "summary"

// newSummaryCommand builds summary: one month's unusually large charges, new recurring charges, net worth and findings.
func newSummaryCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   summaryCommand,
		Short: "Summarize a month: unusual charges, new recurring charges, net worth and findings",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
}
