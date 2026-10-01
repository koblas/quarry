package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// reportFlags is the --since, --until and --account flags shared by the commands that report on a period.
type reportFlags struct {
	since, until string
	accounts     []string
}

// reportFlagHelp is the usage text of the --since, --until and --account flags of one command.
type reportFlagHelp struct {
	since, until, account string
}

// transactionFlagHelp is the flag help of the commands that count transactions in a period.
var transactionFlagHelp = reportFlagHelp{
	since:   "count transactions dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)",
	until:   "count transactions dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)",
	account: "count only the account with this `name` or id; repeat for more",
}

// bind registers --since, --until and --account on cmd, in that order, with help for usage text.
func (w *reportFlags) bind(cmd *cobra.Command, help reportFlagHelp) {
	cmd.Flags().StringVar(&w.since, "since", "", help.since)
	cmd.Flags().StringVar(&w.until, "until", "", help.until)
	cmd.Flags().StringArrayVar(&w.accounts, "account", nil, help.account)
}

// window resolves the flags cmd was given against now, or refuses them as a UsageError.
func (w *reportFlags) window(cmd *cobra.Command, now time.Time) (store.Window, error) {
	var since, until *string
	if cmd.Flags().Changed("since") {
		since = &w.since
	}
	if cmd.Flags().Changed("until") {
		until = &w.until
	}
	window, err := report.ParseWindow(since, until, now)
	if err != nil {
		return store.Window{}, UsageError{msg: err.Error()}
	}
	return window, nil
}
