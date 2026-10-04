package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newHoldingsCommand builds holdings: the securities held in each account today, with their value.
func newHoldingsCommand(_ ReportFactory, _ ConfigLoader, _ func() time.Time, _ *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "holdings",
		Short: "List the securities held in each account and their value",
	}
}
