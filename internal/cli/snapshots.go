package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// snapshotsCommand is the snapshots command's name, for the home-directory refusal and its own usage line.
const snapshotsCommand = "snapshots"

// newSnapshotsCommand builds the snapshots subcommand: list the snapshots
// quarry has taken and mark the one the store was built from.
func newSnapshotsCommand(newSnapshots SnapshotsFactory, loadConfig ConfigLoader) *cobra.Command {
	return &cobra.Command{
		Use:   snapshotsCommand,
		Short: "List the snapshots quarry has taken and which one the store was built from",
		Long: `List the snapshots quarry sync has taken, newest first: when each was
taken, its size, the Quicken file it came from, and which one the store was
built from. Snapshots live in ~/Library/Application Support/quarry/snapshots.

After each successful sync, quarry deletes the oldest snapshots beyond the
newest 12, never the one the store was built from. Snapshots of every
Quicken file count toward the same 12. To keep a different number, set
snapshots.keep in ~/Library/Application Support/quarry/config.toml:

  [snapshots]
  keep = 24

Status is "store" for the snapshot the store was built from, "schema
differs" for one quarry cannot import, and "no manifest" for one that
cannot be used with --from. Rebuild the store from a listed snapshot with
quarry sync --from <ID>.`,
		Example: "  quarry snapshots\n  quarry snapshots prune --dry-run",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: snapshotsCommand + " takes no arguments; to delete old snapshots run quarry snapshots prune; " +
					"Run '" + cmd.CommandPath() + " --help' for usage."}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(snapshotsCommand)
			if err != nil {
				return &runtimeError{err: err}
			}
			printConfigWarnings(cmd, cfg.Warnings)

			srv, err := newSnapshots(cmd.Context(), snapshotsCommand)
			if err != nil {
				return &runtimeError{err: err}
			}
			listing, err := srv.List(cmd.Context())
			if err != nil {
				return &runtimeError{err: err}
			}

			if err := writeResult(cmd, []byte(renderSnapshots(listing))); err != nil {
				return err
			}
			if listing.NoSnapshots != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "quarry: "+listing.NoSnapshots)
			}
			if listing.StoreWarning != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "quarry: warning: "+listing.StoreWarning)
			}
			return nil
		},
	}
}
