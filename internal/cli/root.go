package cli

import (
	"github.com/spf13/cobra"
)

// newRootCommand builds quarry's command tree: a persistent --json flag on
// the root plus the sync subcommand, wired against newServer.
func newRootCommand(newServer ServerFactory, jsonOut *bool) *cobra.Command {
	// No Args or Run field: an unmatched subcommand fails through cobra's
	// own dispatch rather than being accepted as a positional argument.
	root := &cobra.Command{
		Use:   "quarry",
		Short: "Snapshot and query Quicken Classic for Mac data locally",
		Long: `quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. quarry never writes to the Quicken file.`,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(jsonOut, "json", false, "print the result as JSON on stdout")

	root.AddCommand(newSyncCommand(newServer, jsonOut))
	return root
}
