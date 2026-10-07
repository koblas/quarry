package cli

import (
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// claudeNoArgs refuses any argument to the claude child verb.
func claudeNoArgs(verb string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return UsageError{msg: verb + " takes no arguments; Run '" + cmd.CommandPath() + " --help' for usage."}
		}
		return nil
	}
}

// claudeRefuseJSON is a usage error when --json was given to the claude child verb, which prints none.
func claudeRefuseJSON(verb string, jsonOut *bool) error {
	if *jsonOut {
		return UsageError{msg: claudeCommand + " " + verb + " prints no JSON; drop --json"}
	}
	return nil
}

// claudeCommand is the claude group's name, for its children's messages.
const claudeCommand = "claude"

// newClaudeCommand builds the claude group with its install and uninstall children. Bare, it
// prints its help on stdout; an unknown name is a usage error. The children run runTool.
func newClaudeCommand(runTool claudeplugin.Runner, lookPath claudeplugin.LookPath, home string, jsonOut *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   claudeCommand,
		Short: "Install quarry's plugin in Claude Code, or remove it",
		Long: `Install quarry's plugin in Claude Code, or remove it. quarry does this by
running the claude command, so claude must be on your PATH.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newClaudeInstallCommand(runTool, lookPath, home, jsonOut))
	cmd.AddCommand(newClaudeUninstallCommand(runTool, lookPath, home, jsonOut))
	return cmd
}
