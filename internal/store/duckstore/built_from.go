package duckstore

import "context"

// BuiltFrom returns the snapshot path the highest-id import run recorded, or a
// *store.OpenError when the store cannot be read: Missing and OtherFormat
// included (OtherFormat carries the path it could read), no run at all is
// OpenFaultOther.
func (s *Store) BuiltFrom(ctx context.Context) (string, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()

	var path string
	found := false
	err = db.QueryRows(ctx, snapshotPathQuery, nil, func(scan func(dest ...any) error) error {
		found = true
		return scan(&path)
	})
	if err != nil {
		return "", openFault(s.Path(), err)
	}
	if !found {
		return "", openFault(s.Path(), errNoImportRuns)
	}
	return path, nil
}
