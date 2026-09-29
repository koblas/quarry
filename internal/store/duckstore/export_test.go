package duckstore

// The format-check queries openRead runs before a read's own query, so a test fake can tell them apart.
const (
	ColumnExistsQuery  = columnExistsQuery
	FormatVersionQuery = formatVersionQuery
	SnapshotPathQuery  = snapshotPathQuery
)

// FormatCheckQueries lists every query openRead runs before a read's own query.
var FormatCheckQueries = []string{columnExistsQuery, formatVersionQuery, snapshotPathQuery}
