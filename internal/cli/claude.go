package cli

import (
	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/spf13/cobra"
)

// claudeCommand is the claude group's name, for its children's messages.
const claudeCommand = "claude"

// newClaudeCommand builds the claude group: install quarry's plugin in Claude Code, or remove it.
// It runs nothing itself; its children run runTool.
func newClaudeCommand(runTool claudeplugin.Runner, jsonOut *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   claudeCommand,
		Short: "Install quarry's plugin in Claude Code, or remove it",
		Long: `Install quarry's plugin in Claude Code, or remove it. quarry does this by
running the claude command, so claude must be on your PATH.`,
	}
	cmd.AddCommand(newClaudeInstallCommand(runTool, jsonOut))
	return cmd
}
