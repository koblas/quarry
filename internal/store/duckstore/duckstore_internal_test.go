// White-box: build's schema-creation failure has no black-box trigger
// through Replace — a freshly created file never already carries quarry's
// tables. Pre-loading the schema on the same connection before calling
// build directly is the only way to force CREATE TABLE to fail.
package duckstore

import (
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

func Test_build_fails_when_the_schema_already_exists(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "pre-seeded.duckdb")
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(t.Context(), schemaDDL)
	require.NoError(t, err)

	err = build(t.Context(), db, store.Rows{})

	require.Error(t, err)
}
