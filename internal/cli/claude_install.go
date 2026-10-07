package cli

import (
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// installCommand is the install command's name, for its own usage line and messages.
const installCommand = "install"

// newClaudeInstallCommand builds claude install: it refuses arguments, then --json, then has the
// plugin Server add quarry's marketplace and install its plugin, reporting each step on stdout.
func newClaudeInstallCommand(runTool claudeplugin.Runner, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   installCommand,
		Short: "Install quarry's plugin (skill and MCP server) in Claude Code",
		Long: `Install quarry's Claude Code plugin for all your projects. The plugin adds
the quarry skill, which teaches Claude to answer from quarry, and quarry's
MCP server. install runs:

  claude plugin marketplace add --scope user koblas/quarry
  claude plugin install --scope user quarry@quarry

skipping each step that is already done, so running it again is safe.
Claude Code downloads the plugin from github.com/koblas/quarry; quarry
itself sends nothing and opens none of Claude Code's files. The plugin
starts "quarry" from your PATH. Restart Claude Code to load it.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: installCommand + " takes no arguments; Run '" + cmd.CommandPath() + " --help' for usage."}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *jsonOut {
				return UsageError{msg: claudeCommand + " " + installCommand + " prints no JSON; drop --json"}
			}
			srv := claudeplugin.NewServer(claudeplugin.WithRunner(runTool))
			res, err := srv.Install(cmd.Context())
			if err != nil {
				return reportClaudeFailure(cmd, installCommand, renderInstallDone(res), err)
			}
			if err := writeResult(cmd, []byte(renderInstalled(res))); err != nil {
				return err
			}
			if res.UserCopyOff {
				writeClaudeLine(cmd, installCommand, installTurnedOffHint)
			}
			return nil
		},
	}
}
