// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpTestDeadline = 30 * time.Second

// What `go test` binaries report as their main module version.
const mcpTestServerVersion = "(devel)"

func Test_run_mcp_lists_quarrys_tools_over_json_rpc(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	session := peer.session

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, session.Close())
	peer.waitForExit(ctx, t)
	stdout, stderr := peer.stdout, peer.stderr

	assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: mcpTestServerVersion}, session.InitializeResult().ServerInfo)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"query", "describe_schema", "sync_status", "data_quality",
		"spending", "cash_flow", "recurring_charges", "anomalies", "search_transactions",
		"holdings",
	}, names)
	frames := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.GreaterOrEqual(t, len(frames), 2)
	for _, frame := range frames {
		var message struct {
			JSONRPC string `json:"jsonrpc"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame), &message), frame)
		assert.Equal(t, "2.0", message.JSONRPC, frame)
	}
	assert.Empty(t, stderr.String())
}

// mcpPeer is a quarry mcp running in-process with an MCP client connected to it over pipes.
type mcpPeer struct {
	session        *sdk.ClientSession
	exit           <-chan int
	stdout, stderr *bytes.Buffer
}

// startMCP runs quarry mcp over pipes, lets tweak adjust its Env, and connects a client.
func startMCP(ctx context.Context, t *testing.T, tweak func(*cli.Env)) *mcpPeer {
	t.Helper()
	serverStdin, toServer := io.Pipe()
	serverStdout, fromServer := io.Pipe()
	peer := &mcpPeer{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	exit := make(chan int, 1)
	peer.exit = exit
	go func() {
		env := testEnv(io.MultiWriter(fromServer, peer.stdout), peer.stderr)
		env.Stdin = serverStdin
		tweak(&env)
		exit <- runWith(ctx, []string{"mcp"}, env)
		_ = fromServer.Close()
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
	require.NoError(t, err)
	peer.session = session
	return peer
}

// waitForExit returns quarry mcp's exit code, failing the test if it has not exited by ctx's deadline.
func (p *mcpPeer) waitForExit(ctx context.Context, t *testing.T) int {
	t.Helper()
	select {
	case code := <-p.exit:
		return code
	case <-ctx.Done():
		require.FailNow(t, "quarry mcp did not return after the client closed its session")
		return 0
	}
}
