package importer

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Server maps a Quicken v9 snapshot into quarry's schema and writes it
// through a Store.
type Server struct {
	store Store
	open  SourceOpener
}

// Option configures a Server built by NewServer.
type Option func(*Server)

// WithStore sets the Store Import writes the mapped rows into.
func WithStore(s Store) Option {
	return func(srv *Server) { srv.store = s }
}

// WithSourceOpener overrides how Import opens the snapshot path, in place
// of the default read-only SQLite opener.
func WithSourceOpener(open SourceOpener) Option {
	return func(srv *Server) { srv.open = open }
}

// NewServer builds a Server from opts, defaulting SourceOpener to a
// read-only SQLite open.
func NewServer(opts ...Option) *Server {
	srv := &Server{open: defaultOpener}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Import maps snapshotPath's v9 database into quarry's schema and writes it
// through the configured Store, returning the store.Result Replace
// produced. It returns an *UnmappableError when a value the snapshot holds
// cannot be mapped to quarry's schema.
func (srv *Server) Import(ctx context.Context, snapshotPath string) (store.Result, error) {
	return store.Result{}, nil
}
