package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/spf13/cobra"
)

// windowFlags is the --since/--until pair shared by the commands that read a period.
type windowFlags struct {
	since, until string
}

// bind registers --since and --until on cmd.
func (w *windowFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&w.since, "since", "",
		"count transactions dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)")
	cmd.Flags().StringVar(&w.until, "until", "",
		"count transactions dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)")
}

// window resolves the flags cmd was given against now, or refuses them as a UsageError.
func (w *windowFlags) window(cmd *cobra.Command, now time.Time) (store.Window, error) {
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
