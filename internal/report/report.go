package report

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// snapshotExt is the file extension a snapshot's ID is stripped of, matched in any letter case.
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

// statusCommand names status in its refusals; it equals the cli command word.
const statusCommand = "status"

// Status describes the store: its origin snapshot, contents and the checks
// sync ran when it built them. It refuses with a RefusalError when
// interrupted or when the store cannot be opened; other store errors are
// returned unchanged.
func (s *Server) Status(ctx context.Context) (store.Status, error) {
	st, err := s.store.Status(ctx)
	if err != nil {
		return store.Status{}, s.readRefusal(ctx, statusCommand, err)
	}
	return st, nil
}

// SnapshotID returns the ID of the snapshot at path: its file name without the .sqlite extension.
func SnapshotID(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	if strings.EqualFold(ext, snapshotExt) {
		return strings.TrimSuffix(name, ext)
	}
	return name
}
