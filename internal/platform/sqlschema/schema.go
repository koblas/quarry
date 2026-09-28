package sqlschema

// Schema maps a table name to its column names, in the order a caller
// collected them. Fingerprint and Compare both sort internally, so callers
// need not pre-sort.
type Schema map[string][]string

// ColumnRef names one column of one table.
type ColumnRef struct {
	Table  string
	Column string
}
