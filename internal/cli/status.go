package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newStatusCommand builds the status subcommand: read the store's own
// description and render it, as JSON when *jsonOut is set.
func newStatusCommand(newReport ReportFactory, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which snapshot the store was built from and what it holds",
		Long: `Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, and the checks sync ran when it built the store.

status reads only quarry's store; it never looks at Quicken. Run quarry sync
to bring the store up to date.`,
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

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderStatusJSON(st) },
				func() string { return renderStatus(st, srv.Home(), time.Now()) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "", nil)
		},
	}
}
