package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/spf13/cobra"
)

// runtimeError marks err as already classified, so Execute returns it
// unchanged instead of treating it as a cobra-native usage error.
type runtimeError struct {
	err error
}

// unreachable: Execute always matches *runtimeError with errors.As and returns e.err directly, so nothing calls Error() on the wrapper itself.
func (e *runtimeError) Error() string { return e.err.Error() }
func (e *runtimeError) Unwrap() error { return e.err }

// emptyQuickenUsageError is cobra's own missing-value message for --quicken.
var emptyQuickenUsageError = UsageError{msg: "flag needs an argument: --quicken; Run 'quarry sync --help' for usage."}

// emptyFromUsageError is cobra's own missing-value message for --from.
var emptyFromUsageError = UsageError{msg: "flag needs an argument: --from; Run 'quarry sync --help' for usage."}

// fromWithQuickenUsageError refuses --from and --quicken together.
var fromWithQuickenUsageError = UsageError{
	msg: "--from and --quicken cannot be used together; --from rebuilds from a snapshot without reading Quicken",
}

// newSyncCommand builds the sync subcommand: resolve or discover the
// bundle, sync it, and render the result.
func newSyncCommand(newServer ServerFactory, jsonOut *bool) *cobra.Command {
	var quickenPath, fromValue string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Snapshot the open Quicken file and rebuild quarry's store from it",
		Long: `Copy the Quicken file's database with SQLite's backup API into
~/Library/Application Support/quarry/snapshots/, check its integrity, and
compare its tables and columns with quarry's reference for Quicken Classic
for Mac v9. A JSON manifest is written next to each snapshot.

quarry then rebuilds its store, ~/Library/Application Support/quarry/quarry.duckdb,
from the snapshot. In every reconciled account, the reconciled transactions
must add up to the balance of its last reconciled statement in Quicken to the
cent, and every transaction must equal the sum of its splits; if a check
fails, the previous store is left unchanged.

Quicken must be running with the file open: it encrypts the database when
the file is closed. quarry only reads the Quicken file; it never writes to it.

Without --quicken, quarry looks for .quicken files in ~/Documents and in
~/Library/Application Support/Quicken/Documents, and uses the one it finds
if there is exactly one.

With --from, quarry rebuilds the store from a snapshot it took earlier and
does not read Quicken at all.`,
		Example: "  quarry sync --quicken ~/Documents/Home.quicken\n  quarry sync --from 20260927T143005Z",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: "sync takes no arguments; pass the file with --quicken <path>"}
			}
			if cmd.Flags().Changed("from") && cmd.Flags().Changed("quicken") {
				return fromWithQuickenUsageError
			}
			if cmd.Flags().Changed("from") && strings.TrimSpace(fromValue) == "" {
				return emptyFromUsageError
			}
			if cmd.Flags().Changed("quicken") && strings.TrimSpace(quickenPath) == "" {
				return emptyQuickenUsageError
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv, err := newServer(cmd.Context())
			if err != nil {
				return &runtimeError{err: err}
			}
			home := srv.Home()

			var outcome snapshot.Outcome
			if cmd.Flags().Changed("from") {
				outcome, err = srv.ImportFrom(cmd.Context(), fromValue)
			} else {
				bundlePath, resolveErr := resolveBundle(cmd, home, quickenPath)
				if resolveErr != nil {
					return &runtimeError{err: resolveErr}
				}
				outcome, err = srv.SyncAndImport(cmd.Context(), bundlePath)
			}
			var mismatch snapshot.MismatchError
			isMismatch := errors.As(err, &mismatch)
			validationFailed := outcome.Store != nil && !outcome.Store.Built
			if err != nil && !isMismatch && !validationFailed {
				return &runtimeError{err: err}
			}

			var output string
			if *jsonOut {
				data, encErr := renderJSON(outcome)
				if encErr != nil {
					// unreachable: renderJSON's own error path is unreachable for any value SyncAndImport builds; see there.
					return &runtimeError{err: encErr}
				}
				output = string(data)
			} else {
				output = renderSuccess(outcome.Manifest, home)
				switch {
				case validationFailed:
					output += renderStoreFailure(*outcome.Store, outcome.StoreExisted, home)
				case outcome.Store != nil:
					output += renderStore(*outcome.Store, home)
				}
			}
			if _, writeErr := fmt.Fprint(cmd.OutOrStdout(), output); writeErr != nil {
				return &runtimeError{err: outcome.StdoutWriteRefusal(home, writeErr)}
			}

			for _, warning := range outcome.Warnings() {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "quarry: warning: "+warning)
			}

			if isMismatch || validationFailed {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&quickenPath, "quicken", "",
		"`path` to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)")
	cmd.Flags().StringVar(&fromValue, "from", "",
		"`snapshot` to rebuild the store from instead of reading Quicken: an ID such as 20260927T143005Z, or the path to its .sqlite file")

	return cmd
}

// resolveBundle returns the bundle a plain sync snapshots: the --quicken
// value when given, else the one bundle discovery finds.
func resolveBundle(cmd *cobra.Command, home, quickenPath string) (string, error) {
	if cmd.Flags().Changed("quicken") {
		return snapshot.ResolveBundlePath(home, quickenPath)
	}
	return snapshot.DiscoverBundle(home)
}
