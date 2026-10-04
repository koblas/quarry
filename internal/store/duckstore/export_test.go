package duckstore

// SchemaDDL and HoldingWalkQuery are the statements CheckShares runs on its scratch database, and HoldingWalkQuery also runs in Replace, so a test fake can fail one.
const (
	SchemaDDL        = schemaDDL
	HoldingWalkQuery = holdingWalkQuery
)

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
	MixedCategoriesQuery   = mixedCategoriesQuery
	OneSidedTransferQuery  = oneSidedTransferQuery
	PayeeVariantsQuery     = payeeVariantsQuery
	SimilarCategoriesQuery = similarCategoriesQuery
	UncategorizedQuery     = uncategorizedQuery
	UnusedCategoryQuery    = unusedCategoryQuery
	UnlinkedTransferQuery  = unlinkedTransferQuery
)

// DuplicateQuery is the duplicate detector's query.
const DuplicateQuery = duplicateQuery

// The rates queries finishBuild runs, so a test fake can fail one of them.
const (
	StoredRatesQuery = storedRatesQuery
	RecordRatesQuery = recordRatesQuery
)
