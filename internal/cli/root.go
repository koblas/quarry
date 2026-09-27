package cli

import (
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/spf13/cobra"
)

// newRootCommand builds quarry's command tree: a persistent --json flag on
// the root plus the sync subcommand, wired against srv and rooted at home.
// The root carries no Args or Run, so an unmatched subcommand fails through
// cobra's own dispatch rather than being accepted as a positional argument.
func newRootCommand(srv *snapshot.Server, home string, jsonOut *bool) *cobra.Command {
	root := &cobra.Command{
		Use:   "quarry",
		Short: "Snapshot and query Quicken Classic for Mac data locally",
		Long: `quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot and checks it against quarry's schema reference. quarry
never writes to the Quicken file.`,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(jsonOut, "json", false, "print the result as JSON on stdout")

	root.AddCommand(newSyncCommand(srv, home, jsonOut))
	return root
}
