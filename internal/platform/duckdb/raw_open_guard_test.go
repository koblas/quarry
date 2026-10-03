package duckdb_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawDuckDBOpen matches database/sql opening DuckDB by its registered driver name.
var rawDuckDBOpen = regexp.MustCompile(`sql\.Open(DB)?\(\s*"duckdb"`)

func Test_no_go_file_outside_this_package_opens_duckdb_through_database_sql(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	found := rawDuckDBOpens(t, root, filepath.Join(root, "internal", "platform", "duckdb"))

	assert.Empty(t, found, "open DuckDB through internal/platform/duckdb so each open gets its own instance cache")
}

func Test_the_raw_open_guard_reports_a_file_that_opens_duckdb_by_name(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	offender := filepath.Join(root, "store", "fixture_test.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(offender), 0o750))
	require.NoError(t, os.WriteFile(offender, []byte(`db, err := sql.Open("duckdb", path)`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ok.go"), []byte(`db, err := sql.Open("sqlite3", path)`), 0o600))

	found := rawDuckDBOpens(t, root, filepath.Join(root, "allowed"))

	assert.Equal(t, []string{offender}, found)
}

// rawDuckDBOpens lists the .go files under root, outside allowed and dot or vendor directories, that open DuckDB by name.
func rawDuckDBOpens(t *testing.T, root, allowed string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipDir(path, root, allowed, entry.Name())
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path) //nolint:gosec // path comes from walking the module tree
		if err != nil {
			return err
		}
		if rawDuckDBOpen.Match(source) {
			found = append(found, path)
		}
		return nil
	})
	require.NoError(t, err)
	return found
}

func skipDir(path, root, allowed, name string) error {
	if path == allowed || (path != root && (name[0] == '.' || name == "vendor")) {
		return filepath.SkipDir
	}
	return nil
}

// moduleRoot is the nearest directory above the test's working directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above the test directory")
		dir = parent
	}
}
