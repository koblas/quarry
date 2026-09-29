// White-box: scopeSchema is unexported and its table set is a combinatorial
// rule (prefix plus two named exclusions) best driven directly rather than
// through a full Sync.
package snapshot

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_scope_keeps_Z_tables_only(t *testing.T) {
	t.Parallel()
	s := sqlschema.Schema{
		"Z_PRIMARYKEY":    {"Z_ENT"},
		"Z_15USERTAGS":    {"Z_15CASHFLOWTRANSACTIONENTRIES"},
		"Z_METADATA":      {"Z_VERSION"},
		"Z_MODELCACHE":    {"Z_CONTENT"},
		"sqlite_sequence": {"name"},
		"ACCOUNTS":        {"id"},
	}

	got := scopeSchema(s)

	assert.Equal(t, sqlschema.Schema{
		"Z_PRIMARYKEY": {"Z_ENT"},
		"Z_15USERTAGS": {"Z_15CASHFLOWTRANSACTIONENTRIES"},
	}, got)
}

// Pinned against the sqlite3 CLI and README: 82 tables, 1,838 columns.
func Test_scope_of_the_reference_has_the_pinned_table_and_column_counts(t *testing.T) {
	t.Parallel()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)

	scoped := scopeSchema(ref)

	columns := 0
	for _, cols := range scoped {
		columns += len(cols)
	}
	assert.Len(t, scoped, 82)
	assert.Equal(t, 1838, columns)
}
