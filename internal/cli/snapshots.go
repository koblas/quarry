package cli

import "github.com/spf13/cobra"

// newSnapshotsCommand builds the snapshots subcommand: list the snapshots
// quarry has taken and mark the one the store was built from.
func newSnapshotsCommand(_ SnapshotsFactory, _ ConfigLoader) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshots",
		Short: "List the snapshots quarry has taken and which one the store was built from",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
}
