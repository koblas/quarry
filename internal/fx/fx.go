package fx

import (
	"context"
	"net/http"

	"github.com/koblas/quarry/internal/store"
)

// Server fetches the exchange rates a store build needs.
type Server struct {
	client *http.Client
	source Source
}

// Option configures a Server.
type Option func(*Server)

// WithHTTPClient sets the client the default Source uses to reach the publisher.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Server) { s.client = c }
}

// WithSource replaces the default Source.
func WithSource(src Source) Option {
	return func(s *Server) { s.source = src }
}

// NewServer returns a Server with the given options applied.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Refresh returns the rates for req.Need not already in req.Have.
func (s *Server) Refresh(context.Context, store.RatesRequest) (store.RatesRefresh, error) {
	return store.RatesRefresh{}, nil
}
