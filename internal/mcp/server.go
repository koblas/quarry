package mcp

import (
	"context"
	"io"
)

// Server is quarry's MCP server.
type Server struct{}

// Option configures a Server.
type Option func(*Server)

// WithVersion sets the version the server reports to clients.
func WithVersion(string) Option { return func(*Server) {} }

// NewServer builds a Server from opts.
func NewServer(...Option) *Server { return &Server{} }

// Serve speaks MCP over stdin and stdout until the client closes stdin or ctx ends.
func (*Server) Serve(context.Context, io.Reader, io.Writer, io.Writer) error { return nil }
