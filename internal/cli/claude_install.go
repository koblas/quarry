package cli

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// installCommand is the install command's name, for its own usage line and messages.
const installCommand = "install"

// newClaudeInstallCommand builds claude install: it refuses arguments, then --json, installs the
// plugin in Claude Code, then adds quarry's MCP server to Claude Desktop when Desktop is on this Mac.
func newClaudeInstallCommand(runTool claudeplugin.Runner, lookPath claudeplugin.LookPath, home string, executable claudedesktop.Executable, jsonOut *bool) *cobra.Command {
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
		Args: claudeNoArgs(installCommand),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := claudeRefuseJSON(installCommand, jsonOut); err != nil {
				return err
			}
			srv := claudeplugin.NewServer(claudeplugin.WithRunner(runTool), claudeplugin.WithLookPath(lookPath))
			res, err := srv.Install(cmd.Context())
			if errors.Is(err, claudeplugin.ErrClaudeNotFound) {
				return installDesktop(cmd, home, executable, lookPath, true)
			}
			if err != nil {
				return reportClaudeFailure(cmd, installCommand, home, installDoneLead(res), renderInstallDone(res), err)
			}
			if err := writeResult(cmd, []byte(renderInstalled(res))); err != nil {
				return err
			}
			if res.UserCopyOff {
				writeClaudeLine(cmd, installCommand, installTurnedOffHint)
			}
			if res.QuarryNotOnPath {
				writeClaudeLine(cmd, installCommand, installQuarryNotOnPathWarning)
			}
			return installDesktop(cmd, home, executable, lookPath, false)
		},
	}
}

// installDesktop adds quarry's MCP server to Claude Desktop and reports it. With codeAbsent it
// leads with the Claude Code skip line, and refuses when Desktop is skipped too.
func installDesktop(cmd *cobra.Command, home string, executable claudedesktop.Executable, lookPath claudeplugin.LookPath, codeAbsent bool) error {
	srv := claudedesktop.NewServer(claudedesktop.WithHome(home), claudedesktop.WithExecutable(executable),
		claudedesktop.WithLookPath(claudedesktop.LookPath(lookPath)))
	res, err := srv.Install(cmd.Context())
	if err == nil && res.Skipped && codeAbsent {
		return refuseNeitherPresent(cmd, installCommand, home, res.Folder, res.NotAFolder)
	}
	if codeAbsent {
		if werr := skipClaudeCode(cmd); werr != nil {
			return werr
		}
	}
	if err != nil {
		return reportDesktopFailure(cmd, installCommand, home, err)
	}
	if err := writeResult(cmd, []byte(renderDesktopInstalled(home, res))); err != nil {
		return err
	}
	if res.PathQuarry != "" {
		writeClaudeLine(cmd, installCommand, fmt.Sprintf(desktopPathQuarryWarning, claudePath(home, res.Command), claudePath(home, res.PathQuarry)))
	}
	return nil
}
