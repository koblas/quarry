package cli

import (
	"github.com/spf13/cobra"
)

// newMCPCommand builds mcp: it hands the command's streams to serve and
// returns serve's error as a runtime failure, never a usage error.
func newMCPCommand(serve MCPServeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve quarry's store to Claude over MCP (stdio)",
		Long: `Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port. Add it to your client's MCP config with the command
"quarry" and the argument "mcp".

The server reads quarry's store; it never runs quarry sync, never prunes
snapshots and never touches Quicken. Each request reads the store as it
is then, so after you run quarry sync the client sees the new data
without a restart. SQL runs read-only and returns at most 500 rows.

Payee names, memos and category names reach the client as Quicken holds
them: quarry does not yet mask account or card numbers written in them.

Tools: describe_schema, query, sync_status, data_quality.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
}
