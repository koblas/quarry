package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
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

// stdoutWriteRefusal reports that a committed snapshot's result could not
// be written to stdout: the snapshot and its manifest stay on disk regardless.
func stdoutWriteRefusal(home, snapshotPath string, err error) error {
	return fmt.Errorf(
		"cannot write the result to stdout: %s; the snapshot is kept at %s and its .json manifest holds the full result",
		err, homepath.Abbreviate(home, snapshotPath))
}

// newSyncCommand builds the sync subcommand: resolve or discover the
// bundle, sync it, and render the result.
func newSyncCommand(newServer ServerFactory, jsonOut *bool) *cobra.Command {
	var quickenPath string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Snapshot the open Quicken file and verify its schema",
		Long: `Copy the Quicken file's database with SQLite's backup API into
~/Library/Application Support/quarry/snapshots/, check its integrity, and
compare its tables and columns with quarry's reference for Quicken Classic
for Mac v9. A JSON manifest is written next to each snapshot.

Quicken must be running with the file open: it encrypts the database when
the file is closed. quarry only reads the Quicken file; it never writes to it.

Without --quicken, quarry looks for .quicken files in ~/Documents and in
~/Library/Application Support/Quicken/Documents, and uses the one it finds
if there is exactly one.`,
		Example: "  quarry sync --quicken ~/Documents/Home.quicken",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: "sync takes no arguments; pass the file with --quicken <path>"}
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

			var bundlePath string
			if cmd.Flags().Changed("quicken") {
				bundlePath, err = snapshot.ResolveBundlePath(home, quickenPath)
			} else {
				bundlePath, err = snapshot.DiscoverBundle(home)
			}
			if err != nil {
				return &runtimeError{err: err}
			}

			manifest, err := srv.Sync(cmd.Context(), bundlePath)
			var mismatch snapshot.MismatchError
			isMismatch := errors.As(err, &mismatch)
			if err != nil && !isMismatch {
				return &runtimeError{err: err}
			}

			var output string
			if *jsonOut {
				data, encErr := manifest.Encode()
				if encErr != nil {
					// unreachable: Manifest.Encode's own error path is unreachable for any value Sync builds; see there.
					return &runtimeError{err: encErr}
				}
				output = string(data)
			} else {
				output = renderSuccess(manifest, home)
			}
			if _, writeErr := fmt.Fprint(cmd.OutOrStdout(), output); writeErr != nil {
				return &runtimeError{err: stdoutWriteRefusal(home, manifest.Snapshot.Path, writeErr)}
			}

			for _, warning := range manifest.Warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "quarry: warning: "+warning)
			}

			if isMismatch {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&quickenPath, "quicken", "",
		"`path` to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)")

	return cmd
}
