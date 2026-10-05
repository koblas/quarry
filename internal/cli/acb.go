package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newAcbCommand builds acb: the adjusted cost base of each security and the capital gains realized each tax year.
func newAcbCommand(_ ReportFactory, _ ConfigLoader, _ func() time.Time, _ *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "acb",
		Short: "Show adjusted cost base and realized capital gains per tax year, in CAD",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
}
