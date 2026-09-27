package cli

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/spf13/cobra"
)

// runtimeError marks err as already classified by sync's RunE, so Execute's
// default branch returns it unchanged instead of treating it as a
// cobra-native usage error.
type runtimeError struct {
	err error
}

func (e *runtimeError) Error() string { return e.err.Error() }
func (e *runtimeError) Unwrap() error { return e.err }

// newSyncCommand builds the sync subcommand: it takes no positional
// arguments, resolves --quicken when given or else discovers the sole
// .quicken bundle in ~/Documents, and writes either the human success block
// or the --json manifest document to stdout.
func newSyncCommand(srv *snapshot.Server, home string, jsonOut *bool) *cobra.Command {
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

Without --quicken, quarry uses the only .quicken file in ~/Documents.`,
		Example: "  quarry sync --quicken ~/Documents/Home.quicken",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: "sync takes no arguments; pass the file with --quicken <path>"}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			var bundlePath string
			var err error
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

			if *jsonOut {
				data, encErr := manifest.Encode()
				if encErr != nil {
					// unreachable: Manifest.Encode's own error path is unreachable for any value Sync builds; see there.
					return &runtimeError{err: encErr}
				}
				_, _ = fmt.Fprint(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), renderSuccess(manifest, home))
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
		"`path` to the .quicken file to snapshot (default: the only one in ~/Documents)")

	return cmd
}
