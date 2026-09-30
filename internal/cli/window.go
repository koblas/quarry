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

// bind registers --since, --until and --account on cmd, in that order.
func (w *reportFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&w.since, "since", "",
		"count transactions dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)")
	cmd.Flags().StringVar(&w.until, "until", "",
		"count transactions dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)")
	cmd.Flags().StringArrayVar(&w.accounts, "account", nil, "count only the account with this `name` or id; repeat for more")
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
