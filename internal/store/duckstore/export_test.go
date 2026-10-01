package duckstore

// The format-check queries openRead runs before a read's own query, so a test fake can tell them apart.
const (
	ColumnExistsQuery  = columnExistsQuery
	FormatVersionQuery = formatVersionQuery
	SnapshotPathQuery  = snapshotPathQuery
)

// FormatCheckQueries lists every query openRead runs before a read's own query.
var FormatCheckQueries = []string{columnExistsQuery, formatVersionQuery, snapshotPathQuery}

// The detector queries build runs, so a test fake can fail one of them.
const (
	MixedCategoriesQuery  = mixedCategoriesQuery
	OneSidedTransferQuery = oneSidedTransferQuery
	UncategorizedQuery    = uncategorizedQuery
	UnlinkedTransferQuery = unlinkedTransferQuery
)

// DuplicateQuery is the duplicate detector's query.
const DuplicateQuery = duplicateQuery
