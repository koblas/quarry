package report

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// snapshotExt is the file extension a snapshot's ID is stripped of.
const snapshotExt = ".sqlite"

// Server answers read commands against one Store.
type Server struct {
	store Store
	home  string
}

// Option configures a Server.
type Option func(*Server)

// WithStore sets the store the Server reads.
func WithStore(s Store) Option {
	return func(x *Server) { x.store = s }
}

// WithHome sets the home directory output abbreviates paths against.
func WithHome(home string) Option {
	return func(x *Server) { x.home = home }
}

// NewServer returns a Server configured by opts. A Server without a Store
// panics on its first read.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Home returns the home directory output abbreviates paths against.
func (s *Server) Home() string { return s.home }

// Status describes the store: its origin snapshot, contents and the checks
// sync ran when it built them. Store errors are returned unchanged.
func (s *Server) Status(ctx context.Context) (store.Status, error) {
	return s.store.Status(ctx) //nolint:wrapcheck // the store's error is final user copy; a prefix would change it
}

// SnapshotID returns the ID of the snapshot at path: its file name without
// the .sqlite extension.
func SnapshotID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), snapshotExt)
}
