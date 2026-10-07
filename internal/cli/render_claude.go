package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/osreason"
	"github.com/koblas/quarry/internal/platform/toolrun"
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
	installQuarryNotOnPathWarning = `warning: the plugin starts "quarry" from your PATH, and your PATH has none; ` +
		"add the directory holding quarry to your PATH"
	installClaudeNotFoundRefusal = "cannot find the claude command on your PATH; install Claude Code, then run quarry claude install again"

	installAddedLead = "added the quarry marketplace, but "
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

// installDoneLead returns the lead of a failure line that follows the marketplace add this run, else "".
func installDoneLead(res claudeplugin.Result) string {
	if res.MarketplaceAdded {
		return installAddedLead
	}
	return ""
}

// claudePath returns p as printed to the user: "~/..." under home, raw when home is unset.
func claudePath(home, p string) string {
	if home == "" {
		return p
	}
	return homepath.Abbreviate(home, p)
}

// claudeCannotRunLine returns the line for a claude file that would not start, led by lead.
func claudeCannotRunLine(verb, home, lead string, start *toolrun.StartError) string {
	return fmt.Sprintf("%scannot run claude at %q (%s); check that it is Claude Code and that you can run it, then run quarry %s %s again",
		lead, claudePath(home, start.Path), osreason.Reason(start.Err), claudeCommand, verb)
}

// writeClaudeLine writes "quarry: claude <verb>: <text>" to cmd's stderr.
func writeClaudeLine(cmd *cobra.Command, verb, text string) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "quarry: %s %s: %s\n", claudeCommand, verb, text)
}

// reportClaudeFailure writes the steps already done, then err's report, and returns ReportedError;
// lead prefixes a line that follows a step that ran, and an unclassified error is a runtime error.
func reportClaudeFailure(cmd *cobra.Command, verb, home, lead, done string, err error) error {
	if done != "" {
		if werr := writeResult(cmd, []byte(done)); werr != nil {
			return werr
		}
	}
	if exit, ok := errors.AsType[*claudeplugin.ExitError](err); ok {
		replayClaudeOutput(cmd, exit.Output)
		writeClaudeLine(cmd, verb, claudeStepFailureLine(verb, lead, exit))
		return ReportedError{}
	}
	if interrupted, ok := errors.AsType[*claudeplugin.InterruptedError](err); ok {
		writeClaudeLine(cmd, verb, "stopped before "+interrupted.Argv+" finished; run quarry "+claudeCommand+" "+verb+" again")
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
	if errors.Is(err, claudeplugin.ErrClaudeNotFound) {
		writeClaudeLine(cmd, verb, installClaudeNotFoundRefusal)
		return ReportedError{}
	}
	if start, ok := errors.AsType[*toolrun.StartError](err); ok {
		writeClaudeLine(cmd, verb, claudeCannotRunLine(verb, home, lead, start))
		return ReportedError{}
	}
	return &runtimeError{err: err}
}

// claudeStepFailureLine returns the line for a claude child that did not exit zero, led by lead:
// how it ended, a pointer to its replayed message when it printed one, and, after a step that ran, the rerun advice.
func claudeStepFailureLine(verb, lead string, exit *claudeplugin.ExitError) string {
	how := "exited with status " + strconv.Itoa(exit.Status)
	if exit.Signal != nil {
		how = "was stopped by signal " + exit.Signal.String()
	}
	line := lead + exit.Argv + " " + how
	if len(exit.Output) > 0 {
		line += "; see its message above"
	}
	if lead != "" {
		line += ", then run quarry " + claudeCommand + " " + verb + " again"
	}
	return line
}

// replayClaudeOutput writes a failed child's captured output to stderr, ending it with a newline.
func replayClaudeOutput(cmd *cobra.Command, output []byte) {
	_, _ = cmd.ErrOrStderr().Write(output)
	if len(output) > 0 && !bytes.HasSuffix(output, []byte("\n")) {
		_, _ = cmd.ErrOrStderr().Write([]byte("\n"))
	}
}
