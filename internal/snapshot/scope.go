package snapshot

import (
	"strings"

	"github.com/koblas/quarry/internal/platform/sqlschema"
)

// scopedTableExclusions names Z-prefixed tables BR-6 excludes from schema
// comparison even though their name starts with Z. Non-Z tables, including
// sqlite_* ones, are excluded by the Z-prefix check itself.
var scopedTableExclusions = map[string]bool{
	"Z_METADATA":   true,
	"Z_MODELCACHE": true,
}

// scopeSchema returns the subset of s that BR-6 compares: every table whose
// name starts with "Z", excluding Z_METADATA and Z_MODELCACHE.
func scopeSchema(s sqlschema.Schema) sqlschema.Schema {
	scoped := make(sqlschema.Schema, len(s))
	for table, cols := range s {
		if !strings.HasPrefix(table, "Z") {
			continue
		}
		if scopedTableExclusions[table] {
			continue
		}
		scoped[table] = cols
	}
	return scoped
}
