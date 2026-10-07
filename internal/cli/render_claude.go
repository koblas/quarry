package cli

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

const (
	marketplaceAddedLine   = "Added the quarry marketplace to Claude Code.\n"
	marketplacePresentLine = "The quarry marketplace is already in Claude Code.\n"
	pluginInstalledLine    = "Installed the quarry plugin (skill and MCP server) for all your projects.\n"
	pluginPresentLine      = "The quarry plugin is already installed for all your projects.\n"
	installRestartLine     = "Restart Claude Code to load it.\n"

	installTurnedOffHint  = "the quarry plugin is installed but turned off; to turn it on, run claude plugin enable quarry@quarry"
	installForeignRefusal = `Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub; ` +
		"remove it with claude plugin marketplace remove quarry, then run quarry claude install again"
)

// renderInstalled returns the stdout of a successful install: one line per step, then the
// restart line when a step ran.
func renderInstalled(res claudeplugin.Result) string {
	marketplace, plugin := marketplacePresentLine, pluginPresentLine
	if res.MarketplaceAdded {
		marketplace = marketplaceAddedLine
	}
	if res.PluginInstalled {
		plugin = pluginInstalledLine
	}
	out := marketplace + plugin
	if res.Ran() {
		out += installRestartLine
	}
	return out
}

// renderInstallDone returns the line for the step a failed install had already run: only the
// marketplace add can have, since the plugin install is the last step.
func renderInstallDone(res claudeplugin.Result) string {
	if res.MarketplaceAdded {
		return marketplaceAddedLine
	}
	return ""
}

// writeClaudeLine writes "quarry: claude <verb>: <text>" to cmd's stderr.
func writeClaudeLine(cmd *cobra.Command, verb, text string) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "quarry: %s %s: %s\n", claudeCommand, verb, text)
}

// reportClaudeFailure writes the steps already done to stdout, then err's report to stderr, and
// returns ReportedError. A runner's own error comes back as a runtime error for the exit mapping to print.
func reportClaudeFailure(cmd *cobra.Command, verb, done string, err error) error {
	if done != "" {
		if werr := writeResult(cmd, []byte(done)); werr != nil {
			return werr
		}
	}
	if exit, ok := errors.AsType[*claudeplugin.ExitError](err); ok {
		replayClaudeOutput(cmd, exit.Output)
		writeClaudeLine(cmd, verb, exit.Error()+"; see its message above")
		return ReportedError{}
	}
	if unreadable, ok := errors.AsType[*claudeplugin.ListUnreadableError](err); ok {
		writeClaudeLine(cmd, verb, unreadable.Error()+
			"; update Claude Code, or run the two commands in quarry claude "+verb+" --help yourself")
		return ReportedError{}
	}
	if errors.Is(err, claudeplugin.ErrForeignMarketplace) {
		writeClaudeLine(cmd, verb, installForeignRefusal)
		return ReportedError{}
	}
	return &runtimeError{err: err}
}

// replayClaudeOutput writes a failed child's captured output to stderr, ending it with a newline.
func replayClaudeOutput(cmd *cobra.Command, output []byte) {
	_, _ = cmd.ErrOrStderr().Write(output)
	if len(output) > 0 && !bytes.HasSuffix(output, []byte("\n")) {
		_, _ = cmd.ErrOrStderr().Write([]byte("\n"))
	}
}
