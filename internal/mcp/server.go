package mcp

import (
	"context"
	"io"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverName is the name quarry reports to MCP clients at initialize.
const serverName = "quarry"

// develVersion is reported when the build carries no module version.
const develVersion = "(devel)"

// commandName is the command the report factory is asked for, as in the home-directory refusal's "run quarry mcp again".
const commandName = "mcp"

// ReportFactory builds the report server one tool call reads through, for command; it is called
// per call, so the store is never held between calls.
type ReportFactory func(ctx context.Context, command string) (*report.Server, error)

// ConfigLoader loads quarry's config for command; it is called per tool call, so edits to the
// config file apply without a restart.
type ConfigLoader func(command string) (config.Config, error)

// Server is quarry's MCP server: it answers one client's tool calls on the
// streams Serve is given.
type Server struct {
	version   string
	newReport ReportFactory
	newConfig ConfigLoader
}

// Option configures a Server.
type Option func(*Server)

// WithVersion sets the version the server reports to clients; "" keeps the
// "(devel)" default.
func WithVersion(version string) Option {
	return func(s *Server) {
		if version != "" {
			s.version = version
		}
	}
}

// WithReport sets the factory the tools read the store through; a Server without one cannot answer them.
func WithReport(newReport ReportFactory) Option {
	return func(s *Server) { s.newReport = newReport }
}

// WithConfig sets the loader the tools read the config through; a Server without one cannot answer sync_status.
func WithConfig(newConfig ConfigLoader) Option {
	return func(s *Server) { s.newConfig = newConfig }
}

// NewServer builds a Server from opts.
func NewServer(opts ...Option) *Server {
	s := &Server{version: develVersion}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Serve speaks MCP as newline-delimited JSON-RPC on stdin and stdout until
// the client closes stdin or ctx ends, and logs each call that ends isError to stderr. It returns the transport's error
// unwrapped: nil at EOF, ctx.Err() when ctx ends, the write error otherwise.
func (s *Server) Serve(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	srv := sdk.NewServer(
		&sdk.Implementation{Name: serverName, Version: s.version},
		&sdk.ServerOptions{Instructions: instructions},
	)
	srv.AddReceivingMiddleware(absentNullArguments, errorLog(stderr))
	s.addTools(srv)
	// The streams belong to the process, so the transport must not close them.
	return srv.Run(ctx, &sdk.IOTransport{ //nolint:wrapcheck // Serve documents the SDK error as returned unwrapped, so callers branch with errors.Is on the transport's own error
		Reader: io.NopCloser(stdin),
		Writer: nopWriteCloser{stdout},
	})
}

// nopWriteCloser adds a Close that leaves the wrapped writer open.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
