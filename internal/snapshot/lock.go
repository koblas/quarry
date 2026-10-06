package snapshot

import "context"

// LockForSync takes the writer lock for a sync and returns the func that
// releases it; with no Locker it takes nothing.
func (s *Server) LockForSync(_ context.Context) (release func(), err error) {
	return func() {}, nil
}
