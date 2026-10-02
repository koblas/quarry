package main

import (
	"context"
	"io"
	"runtime/debug"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newMCPServe_reports_the_build_infos_module_version(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "a release build", info: &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, want: "v9.9.9"},
		{name: "no build info", info: nil, want: mcpTestServerVersion},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			serverStdin, toServer := io.Pipe()
			serverStdout, fromServer := io.Pipe()
			go func() {
				_ = newMCPServe(c.info)(ctx, serverStdin, fromServer, io.Discard, func() {})
				_ = fromServer.Close()
			}()

			client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
			session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
			require.NoError(t, err)
			defer func() { _ = session.Close() }()

			assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: c.want}, session.InitializeResult().ServerInfo)
		})
	}
}
