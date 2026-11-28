package docs_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNoStore = errors.New("the docs tests call no tool")

// mcpTools is every tool quarry mcp lists to a client, with its input schema.
func mcpTools(t *testing.T) []*sdk.Tool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	srv := mcp.NewServer(
		mcp.WithReport(func(context.Context, string) (*report.Server, error) { return nil, errNoStore }),
		mcp.WithConfig(func(string) (config.Config, error) { return config.Config{}, errNoStore }),
	)
	serverStdin, toServer := io.Pipe()
	serverStdout, fromServer := io.Pipe()
	go func() {
		_ = srv.Serve(ctx, serverStdin, fromServer, io.Discard)
		_ = fromServer.Close()
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "docs-test", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
	require.NoError(t, err)
	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)

	return listed.Tools
}

// parameters is the names of the properties of a tool's input schema, sorted.
func parameters(t *testing.T, tool *sdk.Tool) []string {
	t.Helper()
	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))

	return slices.Sorted(maps.Keys(schema.Properties))
}

func Test_every_mcp_tool_and_parameter_is_on_the_mcp_reference(t *testing.T) {
	page := read(t, "reference/mcp-tools.md")
	tools := mcpTools(t)
	require.GreaterOrEqual(t, len(tools), 13, "found too few tools")

	for _, tool := range tools {
		heading := "### `" + tool.Name + "`"
		text, found := pageSection(page, heading)
		if !assert.True(t, found, "docs/reference/mcp-tools.md has no heading %s", heading) {
			continue
		}
		for _, param := range parameters(t, tool) {
			assert.Contains(t, text, "`"+param+"`", "%s: parameter %s is not under its heading", heading, param)
		}
	}
}
