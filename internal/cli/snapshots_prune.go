package cli

import (
	"fmt"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/spf13/cobra"
)

const (
	// pruneCommand is the prune command's name, for its own usage line.
	pruneCommand = "prune"
	// pruneFactoryCommand names prune to the snapshots factory, for the home-directory refusal.
	pruneFactoryCommand = snapshotsCommand + " " + pruneCommand
	// keepFlag is the flag holding how many snapshots prune keeps.
	keepFlag = "keep"
	// dryRunFlag is the flag that has prune list what it would delete instead of deleting it.
	dryRunFlag = "dry-run"
)

// newPruneCommand builds the snapshots prune subcommand: delete all but the newest
// --keep snapshots (snapshots.keep from the config file when the flag is not given), never the store's own.
// With --dry-run it lists what it would delete and deletes nothing.
func newPruneCommand(newSnapshots SnapshotsFactory, loadConfig ConfigLoader, jsonOut *bool) *cobra.Command {
	var keep int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   pruneCommand,
		Short: "Delete all but the newest snapshots",
		Long: `Delete all but the newest snapshots now, as sync does after each successful
sync. prune keeps --keep snapshots, or snapshots.keep from
~/Library/Application Support/quarry/config.toml (12 unless set). The
snapshot the store was built from is never deleted, even when it is older.

With --dry-run, prune lists what it would delete and deletes nothing.`,
		Example: "  quarry snapshots prune --dry-run\n  quarry snapshots prune --keep 3",
		Args: func(cmd *cobra.Command, args []string) error {
			hint := "Run '" + cmd.CommandPath() + " --help' for usage."
			if len(args) > 0 {
				return UsageError{msg: pruneCommand + " takes no arguments; " + hint}
			}
			if cmd.Flags().Changed(keepFlag) && keep < 1 {
				return UsageError{msg: "--" + keepFlag + " must be 1 or more; the snapshot the store was built from is always kept; " + hint}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(pruneFactoryCommand)
			if err != nil {
				return &runtimeError{err: err}
			}
			printConfigWarnings(cmd, cfg.Warnings)

			limit := cfg.Keep
			if cmd.Flags().Changed(keepFlag) {
				limit = keep
			}

			srv, err := newSnapshots(cmd.Context(), pruneFactoryCommand)
			if err != nil {
				return &runtimeError{err: err}
			}
			prune := srv.Prune
			if dryRun {
				prune = srv.PlanPrune
			}
			pruned, pruneErr := prune(cmd.Context(), limit)
			// Prune returns a zero Pruned with every refusal that leaves nothing to report.
			if pruneErr != nil && pruned.Keep == 0 {
				return &runtimeError{err: pruneErr}
			}
			return reportPruned(cmd, srv, pruned, pruneErr, *jsonOut, cfg.WarningsAbsolute)
		},
	}
	cmd.Flags().IntVar(&keep, keepFlag, 0, "keep the newest `n` snapshots (default: snapshots.keep in the config file, 12 unless set)")
	cmd.Flags().BoolVar(&dryRun, dryRunFlag, false, "list the snapshots prune would delete without deleting them")
	return cmd
}

// reportPruned writes what pruned deleted to stdout, as the --json document when asJSON, then a line per
// failed delete to stderr. It returns pruneErr, the run's own refusal, else ReportedError when a delete failed.
func reportPruned(cmd *cobra.Command, srv *snapshot.Server, pruned snapshot.Pruned, pruneErr error, asJSON bool, warnings []string) error {
	out, err := renderResult(asJSON,
		func() ([]byte, error) { return renderPrunedJSON(pruned, warnings) },
		func() string { return renderPruned(pruned, srv.Home()) })
	if err != nil {
		// unreachable: renderResult fails only via marshalDocument, and the prune document holds strings, ints, bools and slices; see marshalDocument.
		return err
	}
	if err := writeResult(cmd, out); err != nil {
		return err
	}
	for _, failure := range pruned.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "quarry: cannot delete snapshot %s: %s\n", failure.Entry.ID, failure.Reason)
	}
	if pruneErr != nil {
		return &runtimeError{err: pruneErr}
	}
	if len(pruned.Failed) > 0 {
		return ReportedError{}
	}
	return nil
}
