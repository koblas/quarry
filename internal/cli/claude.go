package cli

import (
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

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
