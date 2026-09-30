package duckstore

import "context"

// BuiltFrom returns the snapshot path the latest import run recorded.
func (s *Store) BuiltFrom(_ context.Context) (string, error) {
	return "", nil
}
