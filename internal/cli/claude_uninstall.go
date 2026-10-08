package cli

import (
	"errors"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// uninstallCommand is the uninstall command's name, for its own usage line and messages.
const uninstallCommand = "uninstall"

// newClaudeUninstallCommand builds claude uninstall: it refuses arguments, then --json, then has the
// plugin Server uninstall quarry's plugin and remove its marketplace, then removes quarry from
// Claude Desktop, reporting each step on stdout.
func newClaudeUninstallCommand(runTool claudeplugin.Runner, lookPath claudeplugin.LookPath, home string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   uninstallCommand,
		Short: "Uninstall quarry from Claude Code and Claude Desktop",
		Long: `Uninstall quarry from Claude Code and from Claude Desktop, whichever of
the two is on this Mac; uninstall skips the other and says so.

In Claude Code, uninstall removes quarry's plugin from your user scope
and the quarry marketplace. It runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

A copy of the plugin installed for a single project stays, and uninstall
names each one.

In Claude Desktop, uninstall removes the "quarry" entry under mcpServers
in ~/Library/Application Support/Claude/claude_desktop_config.json when
that entry runs quarry mcp. Every other setting stays as it was, though
the file's layout may change, and the file it replaces is saved as
claude_desktop_config.json.before-quarry. Quit Claude Desktop first.

Each step with nothing to remove is skipped. Your quarry store and
snapshots are not touched.`,
		Args: claudeNoArgs(uninstallCommand),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := claudeRefuseJSON(uninstallCommand, jsonOut); err != nil {
				return err
			}
			srv := claudeplugin.NewServer(claudeplugin.WithRunner(runTool), claudeplugin.WithLookPath(lookPath))
			res, err := srv.Uninstall(cmd.Context())
			if errors.Is(err, claudeplugin.ErrClaudeNotFound) {
				return uninstallDesktop(cmd, home, true)
			}
			if err != nil {
				reported := reportClaudeFailure(cmd, uninstallCommand, home, uninstallDoneLead(res), renderUninstallDone(res), err)
				return continueAfterCodeFailure(err, reported, func() error {
					return uninstallDesktop(cmd, home, false)
				})
			}
			if err := writeResult(cmd, []byte(renderUninstalled(res))); err != nil {
				return err
			}
			for _, hint := range uninstallRemainingHints(home, res) {
				writeClaudeLine(cmd, uninstallCommand, hint)
			}
			return uninstallDesktop(cmd, home, false)
		},
	}
}

// uninstallDesktop removes quarry's MCP server from Claude Desktop and reports the outcome on stdout.
// With codeAbsent it leads with the Claude Code skip line, and refuses when Desktop is skipped too.
func uninstallDesktop(cmd *cobra.Command, home string, codeAbsent bool) error {
	res, err := claudedesktop.NewServer(claudedesktop.WithHome(home)).Uninstall(cmd.Context())
	if err == nil && res.Skipped && codeAbsent {
		return refuseNeitherPresent(cmd, uninstallCommand, home, res.Folder, res.NotAFolder)
	}
	if codeAbsent {
		if werr := skipClaudeCode(cmd); werr != nil {
			return werr
		}
	}
	if err != nil {
		return reportDesktopFailure(cmd, uninstallCommand, home, err)
	}
	return writeResult(cmd, []byte(renderDesktopUninstalled(home, res)))
}
