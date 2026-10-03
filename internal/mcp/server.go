package mcp

import (
	"context"
	"errors"
	"io"
	"time"

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

// errIncomplete is Serve's refusal of a Server built without the report factory or config loader it reads through.
var errIncomplete = errors.New("mcp: Server needs WithReport and WithConfig")

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
	timeout   time.Duration
	now       func() time.Time
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

// WithReport sets the factory the tools read the store through; Serve requires it.
func WithReport(newReport ReportFactory) Option {
	return func(s *Server) { s.newReport = newReport }
}

// WithConfig sets the loader the tools read the config through; Serve requires it.
func WithConfig(newConfig ConfigLoader) Option {
	return func(s *Server) { s.newConfig = newConfig }
}

// WithTimeout sets the deadline of each tool call, in whole seconds.
func WithTimeout(d time.Duration) Option {
	return func(s *Server) { s.timeout = d }
}

// WithClock sets the clock the tools read "today" from, once per call.
func WithClock(now func() time.Time) Option {
	return func(s *Server) { s.now = now }
}

// NewServer builds a Server from opts.
func NewServer(opts ...Option) *Server {
	s := &Server{version: develVersion, timeout: callTimeout, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Serve speaks MCP as newline-delimited JSON-RPC on stdin and stdout until the client closes stdin or ctx ends,
// logging each isError call to stderr. It returns the transport's error unwrapped (nil at EOF, ctx.Err() when
// ctx ends), or errIncomplete when WithReport or WithConfig is missing.
func (s *Server) Serve(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	if s.newReport == nil || s.newConfig == nil {
		return errIncomplete
	}
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
