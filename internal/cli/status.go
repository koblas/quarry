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
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv, err := newReport(cmd.Context(), cmd.Name())
			if err != nil {
				return &runtimeError{err: err}
			}

			st, err := srv.Status(cmd.Context())
			if err != nil {
				return &runtimeError{err: err}
			}

			var out []byte
			if *jsonOut {
				if out, err = renderStatusJSON(st); err != nil {
					// unreachable: renderStatusJSON's own error path is unreachable for any Status; see there.
					return &runtimeError{err: err}
				}
			} else {
				out = []byte(renderStatus(st, srv.Home(), time.Now()))
			}

			if _, err := cmd.OutOrStdout().Write(out); err != nil {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
}
