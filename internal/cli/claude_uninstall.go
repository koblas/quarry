package cli

import (
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// uninstallCommand is the uninstall command's name, for its own usage line and messages.
const uninstallCommand = "uninstall"

// newClaudeUninstallCommand builds claude uninstall: it refuses arguments, then --json, then has the
// plugin Server uninstall quarry's plugin and remove its marketplace, reporting each step on stdout.
func newClaudeUninstallCommand(runTool claudeplugin.Runner, lookPath claudeplugin.LookPath, home string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   uninstallCommand,
		Short: "Uninstall quarry's plugin from Claude Code",
		Long: `Uninstall quarry's Claude Code plugin from your user scope and remove the
quarry marketplace. uninstall runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

skipping each step with nothing to remove. A copy of the plugin installed
for a single project stays, and uninstall names each one. Your quarry
store and snapshots are not touched.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: uninstallCommand + " takes no arguments; Run '" + cmd.CommandPath() + " --help' for usage."}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *jsonOut {
				return UsageError{msg: claudeCommand + " " + uninstallCommand + " prints no JSON; drop --json"}
			}
			srv := claudeplugin.NewServer(claudeplugin.WithRunner(runTool), claudeplugin.WithLookPath(lookPath))
			res, err := srv.Uninstall(cmd.Context())
			if err != nil {
				return reportClaudeFailure(cmd, uninstallCommand, home, uninstallDoneLead(res), renderUninstallDone(res), err)
			}
			return writeResult(cmd, []byte(renderUninstalled(res)))
		},
	}
}
