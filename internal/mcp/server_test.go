package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const serveDeadline = 30 * time.Second

var errClientGone = errors.New("client gone")

// failingWriter fails every write with errClientGone.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errClientGone }

// running is a Serve call wired to an in-process MCP client.
type running struct {
	session *sdk.ClientSession
	served  <-chan error
}

// startServer serves srv to a connected client and returns once the client has initialized.
func startServer(t *testing.T, srv *mcp.Server) running {
	t.Helper()
	return startServerLogging(t, srv, io.Discard)
}

// startServerLogging is startServer with the server's stderr going to stderr.
func startServerLogging(t *testing.T, srv *mcp.Server, stderr io.Writer) running {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), serveDeadline)
	t.Cleanup(cancel)
	serverStdin, toServer := io.Pipe()
	serverStdout, fromServer := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(ctx, serverStdin, fromServer, stderr)
		_ = fromServer.Close()
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
	require.NoError(t, err)
	return running{session: session, served: served}
}

// result waits for Serve to return.
func (r running) result(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.served:
		return err
	case <-time.After(serveDeadline):
		require.FailNow(t, "Serve did not return")
		return nil
	}
}

func Test_serve_identifies_an_unversioned_build_as_devel(t *testing.T) {
	cases := []struct {
		name    string
		options []mcp.Option
		want    string
	}{
		{name: "no version option", want: "(devel)"},
		{name: "empty version", options: []mcp.Option{mcp.WithVersion("")}, want: "(devel)"},
		{name: "a release version", options: []mcp.Option{mcp.WithVersion("v1.2.3")}, want: "v1.2.3"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := startServer(t, mcp.NewServer(c.options...))

			got := r.session.InitializeResult().ServerInfo

			assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: c.want}, got)
		})
	}
}

func Test_serve_returns_nil_when_the_client_closes_stdin(t *testing.T) {
	r := startServer(t, mcp.NewServer())

	require.NoError(t, r.session.Close())

	assert.NoError(t, r.result(t))
}

func Test_serve_returns_the_write_error_when_stdout_fails(t *testing.T) {
	serverStdin, toServer := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- mcp.NewServer().Serve(t.Context(), serverStdin, failingWriter{}, io.Discard)
	}()

	_, err := toServer.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"))
	require.NoError(t, err)

	select {
	case got := <-served:
		assert.Same(t, errClientGone, got)
	case <-time.After(serveDeadline):
		require.FailNow(t, "Serve did not return after stdout failed")
	}
}

func Test_serve_returns_the_context_error_when_the_context_ends(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	serverStdin, _ := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- mcp.NewServer().Serve(ctx, serverStdin, io.Discard, io.Discard)
	}()

	cancel()

	select {
	case got := <-served:
		require.ErrorIs(t, got, context.Canceled)
	case <-time.After(serveDeadline):
		require.FailNow(t, "Serve did not return after its context ended")
	}
}

func Test_a_tool_call_with_absent_null_or_empty_arguments_reaches_the_handler(t *testing.T) {
	r := startServer(t, mcp.NewServer())

	for name, args := range map[string]any{
		"omitted": nil,
		"null":    json.RawMessage("null"),
		"empty":   map[string]any{},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := r.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "data_quality", Arguments: args})

			require.NoError(t, err)
			require.NotEmpty(t, got.Content)
			assert.Equal(t, &sdk.TextContent{Text: "this tool is not available yet"}, got.Content[0])
		})
	}
}

func Test_an_unbuilt_tool_answers_isError(t *testing.T) {
	r := startServer(t, mcp.NewServer())

	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"data_quality", nil},
	} {
		t.Run(tool.name, func(t *testing.T) {
			got, err := r.session.CallTool(t.Context(), &sdk.CallToolParams{Name: tool.name, Arguments: tool.args})

			require.NoError(t, err)
			assert.True(t, got.IsError)
			require.NotEmpty(t, got.Content)
			assert.Equal(t, &sdk.TextContent{Text: "this tool is not available yet"}, got.Content[0])
		})
	}
}
