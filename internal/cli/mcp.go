package cli

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/spf13/cobra"
)

const (
	mcpJSONRefusal  = "mcp always speaks JSON on stdout; drop --json"
	mcpTerminalHint = "quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop."
)

// newMCPCommand builds mcp: it refuses --json and runs serve on the command's streams,
// returning its failure as a runtime error; at a terminal, serve's ready call prints a hint.
func newMCPCommand(serve MCPServeFunc, isTerminal TerminalProbe, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Args:  noArgs,
		Short: "Serve quarry's store to Claude over MCP (stdio)",
		Long: `Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port. Add it to your client's MCP config with the command
"quarry" and the argument "mcp".

The server reads quarry's store; it never runs quarry sync, never prunes
snapshots and never touches Quicken. Each request reads the store as it
is then, so after you run quarry sync the client sees the new data
without a restart. SQL runs read-only, and every list a tool returns
stops at 500 entries.

Payee names, memos, account names and category names reach the client
as Quicken holds them; quarry does not rewrite or mask them.

Tools: describe_schema, query, sync_status, data_quality, spending,
cash_flow, recurring_charges, anomalies.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *jsonOut {
				return UsageError{msg: mcpJSONRefusal}
			}
			stdin, stderr := cmd.InOrStdin(), cmd.ErrOrStderr()
			ready := func() {
				if isTerminal != nil && isTerminal(stdin) {
					_, _ = fmt.Fprintln(stderr, mcpTerminalHint)
				}
			}
			if err := serve(cmd.Context(), stdin, cmd.OutOrStdout(), stderr, ready); !mcpStopped(err) {
				return &runtimeError{err: err}
			}
			return nil
		},
	}
}

// mcpStopped reports whether err from serving is a normal stop: the client
// closed stdin (nil), the process was told to stop, or the client stopped reading stdout.
func mcpStopped(err error) bool {
	return err == nil || errors.Is(err, context.Canceled) || errors.Is(err, syscall.EPIPE)
}
