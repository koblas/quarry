package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/claudedesktop"
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

	desktopQuitLine   = "Quit and reopen Claude Desktop to load it.\n"
	desktopAddedFmt   = "Added the quarry MCP server to Claude Desktop; it starts %q.\n"
	desktopKeptFmt    = "The quarry MCP server is already in Claude Desktop; it starts %q.\n"
	desktopUpdatedFmt = "Updated the quarry MCP server in Claude Desktop to start %q instead of %q.\n"
	desktopSkippedFmt = "Skipped Claude Desktop: %q does not exist.\n"
	desktopNoHome     = "cannot find your home directory ($HOME is not set), so quarry cannot look for Claude Desktop; " +
		"set HOME, then run quarry claude %s again"

	desktopRemovedLine      = "Removed the quarry MCP server from Claude Desktop.\n"
	desktopAbsentLine       = "The quarry MCP server is not in Claude Desktop.\n"
	desktopQuitUnloadLine   = "Quit and reopen Claude Desktop to unload it.\n"
	desktopForeignUninstall = `Claude Desktop has an MCP server named "quarry" that does not run quarry mcp, ` +
		"so quarry leaves it alone; to remove it, delete it from %q yourself"
	desktopSymlinkUninstall = `%q is a symbolic link, so quarry leaves it alone; ` +
		`if it has a "quarry" entry under mcpServers, remove it yourself`

	desktopPathQuarryWarning = `warning: Claude Desktop starts %q, but the quarry on your PATH is %q; ` +
		"run quarry claude install with the quarry you want Claude Desktop to start"
	desktopTempRefusal = "this quarry runs from a temporary build (%q), which will be gone when Claude Desktop starts it; " +
		"run quarry claude install from an installed quarry, not go run"
	desktopBinaryNameRefusal = "this quarry binary is named %q, and quarry recognises its Claude Desktop entry only when the binary is named quarry; " +
		"rename it to quarry, or put a link named quarry to it on your PATH, then run quarry claude install again"
	desktopNoBinaryRefusal = "cannot tell where this quarry binary is (%s), so quarry cannot add it to Claude Desktop; " +
		"add %s under mcpServers in %q yourself"
	desktopPathQuarryPlaceholder = "<the path command -v quarry prints>"

	// The backup and write refusals share their fix clause: the config folder, then the verb.
	desktopBackupRefusal  = "cannot save %q (%s), so %q is unchanged" + desktopFixFolder
	desktopWriteRefusal   = "cannot write %q (%s), so it is unchanged" + desktopFixFolder
	desktopFixFolder      = "; check the permissions of %q, then run quarry claude %s again"
	desktopForeignRefusal = `Claude Desktop has an MCP server named "quarry" that does not run quarry mcp, ` +
		"so quarry leaves it alone; rename or remove it in %q, then run quarry claude %s again"

	// The three unusable-content refusals share their fix clause: repair the file, then the verb.
	desktopInvalidJSONRefusal = "cannot read %q: it is not valid JSON (%s%s)" + desktopFixJSON
	desktopTopLevelRefusal    = "cannot add quarry to %q: it holds a JSON %s, not an object" + desktopFixJSON
	desktopServersRefusal     = "cannot add quarry to %q: its mcpServers is a JSON %s, not an object" + desktopFixJSON
	desktopFixJSON            = "; fix it so Claude Desktop can read it too, then run quarry claude %s again"
	desktopSymlinkRefusal     = "%q is a symbolic link, so quarry leaves it alone; add %s under mcpServers " +
		"in the file it links to yourself, then quit and reopen Claude Desktop"
	desktopNotAFileRefusal = "%q is not a file, so quarry leaves it alone; move it aside, then run quarry claude install again"
	desktopReadRefusal     = "cannot read %q (%s); check its permissions, then run quarry claude %s again"

	pluginUninstalledLine  = "Uninstalled the quarry plugin from Claude Code.\n"
	pluginAbsentLine       = "The quarry plugin is not installed for all your projects.\n"
	marketplaceRemovedLine = "Removed the quarry marketplace from Claude Code.\n"
	marketplaceAbsentLine  = "The quarry marketplace is not in Claude Code.\n"
	marketplaceKeptLine    = "Kept the quarry marketplace: the quarry plugin is still installed elsewhere and needs it.\n"
	uninstallRestartLine   = "Restart Claude Code to unload it.\n"

	uninstallForeignRefusal = `Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub, ` +
		"so quarry leaves it and its plugin alone; to remove them, run claude plugin uninstall quarry@quarry, " +
		"then claude plugin marketplace remove quarry"
	uninstallClaudeNotFoundRefusal = "cannot find the claude command on your PATH; add it to your PATH, then run quarry claude uninstall again"

	uninstallRemovedLead = "uninstalled the quarry plugin, but "
)

// claudeRefusals is the copy of the two refusals whose wording depends on the verb.
type claudeRefusals struct {
	foreign  string // a marketplace named quarry that is not quarry's
	notFound string // no claude command on PATH
}

// claudeRefusalCopy holds each verb's refusal copy, keyed by verb.
var claudeRefusalCopy = map[string]claudeRefusals{
	installCommand:   {foreign: installForeignRefusal, notFound: installClaudeNotFoundRefusal},
	uninstallCommand: {foreign: uninstallForeignRefusal, notFound: uninstallClaudeNotFoundRefusal},
}

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

// renderUninstalled returns the stdout of a successful uninstall: one line per step, then the
// restart line when a step ran.
func renderUninstalled(res claudeplugin.UninstallResult) string {
	plugin, marketplace := pluginAbsentLine, marketplaceAbsentLine
	if res.PluginUninstalled {
		plugin = pluginUninstalledLine
	}
	switch {
	case res.MarketplaceKept:
		marketplace = marketplaceKeptLine
	case res.MarketplaceRemoved:
		marketplace = marketplaceRemovedLine
	}
	out := plugin + marketplace
	if res.Ran() {
		out += uninstallRestartLine
	}
	return out
}

// uninstallRemainingHints returns one stderr hint per copy the uninstall left, in list order.
func uninstallRemainingHints(home string, res claudeplugin.UninstallResult) []string {
	hints := make([]string, 0, len(res.Remaining))
	for _, c := range res.Remaining {
		hints = append(hints, remainingCopyHint(home, c))
	}
	return hints
}

// remainingCopyHint returns the hint for one copy. A project or local copy gets the removal command
// with the scope claude reported and, when claude named one, the project path as claudePath prints
// it; any other scope is one claude plugin uninstall cannot remove from.
func remainingCopyHint(home string, c claudeplugin.Copy) string {
	if c.Scope != "project" && c.Scope != "local" {
		return fmt.Sprintf("the quarry plugin is still installed at scope %q, which claude plugin uninstall cannot remove from; "+
			"it stays until whoever manages that scope removes it", c.Scope)
	}
	remove := "to remove it, run claude plugin uninstall --scope " + c.Scope + " quarry@quarry"
	if c.ProjectPath == "" {
		return "the quarry plugin is still installed for a project Claude Code did not name; " + remove + " in that project's directory"
	}
	return fmt.Sprintf("the quarry plugin is still installed for project %q; %s in that directory", claudePath(home, c.ProjectPath), remove)
}

// renderUninstallDone returns the line for the step a failed uninstall had already run: only the
// plugin uninstall can have, since the marketplace remove is the last step.
func renderUninstallDone(res claudeplugin.UninstallResult) string {
	if res.PluginUninstalled {
		return pluginUninstalledLine
	}
	return ""
}

// uninstallDoneLead returns the lead of a failure line that follows the plugin uninstall this run, else "".
func uninstallDoneLead(res claudeplugin.UninstallResult) string {
	if res.PluginUninstalled {
		return uninstallRemovedLead
	}
	return ""
}

// renderDesktopInstalled returns the stdout of a Claude Desktop install: the skip line, else the
// line for what happened to the entry plus the quit line when the config changed.
func renderDesktopInstalled(home string, res claudedesktop.Result) string {
	if res.Skipped {
		return fmt.Sprintf(desktopSkippedFmt, claudePath(home, res.Folder))
	}
	started := claudePath(home, res.Command)
	switch res.Outcome {
	case claudedesktop.Updated:
		return fmt.Sprintf(desktopUpdatedFmt, started, claudePath(home, res.Previous)) + desktopQuitLine
	case claudedesktop.Unchanged:
		return fmt.Sprintf(desktopKeptFmt, started)
	case claudedesktop.Added:
	}
	return fmt.Sprintf(desktopAddedFmt, started) + desktopQuitLine
}

// renderDesktopUninstalled returns the stdout of a Claude Desktop uninstall: the skip line, else the
// removal line plus the quit line, else the line saying the entry is not there.
func renderDesktopUninstalled(home string, res claudedesktop.UninstallResult) string {
	switch {
	case res.Skipped:
		return fmt.Sprintf(desktopSkippedFmt, claudePath(home, res.Folder))
	case res.Removed:
		return desktopRemovedLine + desktopQuitUnloadLine
	}
	return desktopAbsentLine
}

// reportDesktopFailure writes err's report for a failed Claude Desktop step and returns
// ReportedError; an unclassified error is a runtime error.
func reportDesktopFailure(cmd *cobra.Command, verb, home string, err error) error {
	line, ok := desktopFailureLine(verb, home, err)
	if !ok {
		return &runtimeError{err: err}
	}
	writeClaudeLine(cmd, verb, line)
	return ReportedError{}
}

// desktopFailureLine returns the stderr text for a classified Claude Desktop failure, else false.
func desktopFailureLine(verb, home string, err error) (string, bool) {
	if errors.Is(err, claudedesktop.ErrNoHome) {
		return fmt.Sprintf(desktopNoHome, verb), true
	}
	if line, ok := desktopBinaryFailureLine(home, err); ok {
		return line, true
	}
	if line, ok := desktopWriteFailureLine(verb, home, err); ok {
		return line, true
	}
	return desktopConfigFailureLine(verb, home, err)
}

// desktopBinaryFailureLine classifies the failures of learning which quarry binary Desktop would start.
func desktopBinaryFailureLine(home string, err error) (string, bool) {
	if temp, ok := errors.AsType[*claudedesktop.TempBuildError](err); ok {
		return fmt.Sprintf(desktopTempRefusal, claudePath(home, temp.Path)), true
	}
	if name, ok := errors.AsType[*claudedesktop.BinaryNameError](err); ok {
		return fmt.Sprintf(desktopBinaryNameRefusal, name.Base), true
	}
	if exe, ok := errors.AsType[*claudedesktop.ExecutableError](err); ok {
		return fmt.Sprintf(desktopNoBinaryRefusal, osreason.Reason(exe.Err),
			desktopEntryJSON(desktopPathQuarryPlaceholder), claudePath(home, exe.Config)), true
	}
	return "", false
}

// desktopWriteFailureLine classifies the failures of saving the backup and writing the config.
func desktopWriteFailureLine(verb, home string, err error) (string, bool) {
	if backup, ok := errors.AsType[*claudedesktop.BackupError](err); ok {
		return fmt.Sprintf(desktopBackupRefusal, claudePath(home, backup.Path), osreason.Reason(backup.Err),
			claudePath(home, backup.Config), claudePath(home, filepath.Dir(backup.Path)), verb), true
	}
	if write, ok := errors.AsType[*claudedesktop.WriteError](err); ok {
		return fmt.Sprintf(desktopWriteRefusal, claudePath(home, write.Path), osreason.Reason(write.Err),
			claudePath(home, filepath.Dir(write.Path)), verb), true
	}
	return "", false
}

// desktopConfigFailureLine classifies the failures of reading and understanding the config.
func desktopConfigFailureLine(verb, home string, err error) (string, bool) {
	if foreign, ok := errors.AsType[*claudedesktop.ForeignEntryError](err); ok {
		return desktopForeignLine(verb, claudePath(home, foreign.Path)), true
	}
	if link, ok := errors.AsType[*claudedesktop.SymlinkError](err); ok {
		return desktopSymlinkLine(verb, claudePath(home, link.Path), link.Command), true
	}
	if notFile, ok := errors.AsType[*claudedesktop.NotAFileError](err); ok {
		return fmt.Sprintf(desktopNotAFileRefusal, claudePath(home, notFile.Path)), true
	}
	if read, ok := errors.AsType[*claudedesktop.ReadError](err); ok {
		return fmt.Sprintf(desktopReadRefusal, claudePath(home, read.Path), osreason.Reason(read.Err), verb), true
	}
	if bad, ok := errors.AsType[*claudedesktop.InvalidJSONError](err); ok {
		return fmt.Sprintf(desktopInvalidJSONRefusal, claudePath(home, bad.Path), bad.Err, invalidJSONDetail(bad.Err), verb), true
	}
	if top, ok := errors.AsType[*claudedesktop.TopLevelError](err); ok {
		return fmt.Sprintf(desktopTopLevelRefusal, claudePath(home, top.Path), top.Kind, installCommand), true
	}
	if servers, ok := errors.AsType[*claudedesktop.ServersError](err); ok {
		return fmt.Sprintf(desktopServersRefusal, claudePath(home, servers.Path), servers.Kind, installCommand), true
	}
	return "", false
}

// desktopForeignLine returns the refusal for a quarry entry that does not run quarry mcp: install
// asks for it to be renamed, uninstall leaves its removal to the user.
func desktopForeignLine(verb, config string) string {
	if verb == uninstallCommand {
		return fmt.Sprintf(desktopForeignUninstall, config)
	}
	return fmt.Sprintf(desktopForeignRefusal, config, verb)
}

// desktopSymlinkLine returns the refusal for a config that is a symbolic link: install names the entry
// to add by hand, uninstall the entry to remove.
func desktopSymlinkLine(verb, config, command string) string {
	if verb == uninstallCommand {
		return fmt.Sprintf(desktopSymlinkUninstall, config)
	}
	return fmt.Sprintf(desktopSymlinkRefusal, config, desktopEntryJSON(command))
}

// invalidJSONDetail returns ", at byte N" for a syntax error, else "".
func invalidJSONDetail(err error) string {
	if syntax, ok := errors.AsType[*json.SyntaxError](err); ok {
		return ", at byte " + strconv.FormatInt(syntax.Offset, 10)
	}
	return ""
}

// desktopEntryJSON returns the quarry entry for a Claude Desktop config in its one-line form, with
// command as the absolute path: a pasted "~" would not expand in JSON.
func desktopEntryJSON(command string) string {
	var quoted bytes.Buffer
	enc := json.NewEncoder(&quoted)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(command); err != nil {
		// unreachable: encoding a Go string cannot fail and bytes.Buffer.Write never returns an error
		quoted.Reset()
		quoted.WriteString(strconv.Quote(command))
	}
	return `"quarry": {"command": ` + strings.TrimSuffix(quoted.String(), "\n") + `, "args": ["mcp"]}`
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
		writeClaudeLine(cmd, verb, claudeRefusalCopy[verb].foreign)
		return ReportedError{}
	}
	if errors.Is(err, claudeplugin.ErrClaudeNotFound) {
		writeClaudeLine(cmd, verb, claudeRefusalCopy[verb].notFound)
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
