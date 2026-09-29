package v9fixture

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/require"
)

// referenceTemplate holds the bytes of a closed, non-WAL SQLite database
// carrying v9.ReferenceDDL's schema with no rows, built once per test binary
// so every fixture function copies it instead of re-running the DDL's ~80
// CREATE TABLE statements against a fresh file.
var (
	referenceTemplateOnce  sync.Once
	referenceTemplateBytes []byte
	referenceTemplateErr   error
)

// referenceTemplate returns the reference schema template's bytes, building
// it on the first call and reusing the result for every later one.
func referenceTemplate(tb testing.TB) []byte {
	tb.Helper()
	referenceTemplateOnce.Do(func() {
		referenceTemplateBytes, referenceTemplateErr = buildReferenceTemplate()
	})
	require.NoError(tb, referenceTemplateErr)
	return referenceTemplateBytes
}

// buildReferenceTemplate creates a fresh SQLite file under a scratch
// directory, runs v9.ReferenceDDL against it, and returns the resulting
// file's bytes.
func buildReferenceTemplate() ([]byte, error) {
	dir, err := os.MkdirTemp("", "v9fixture-template")
	if err != nil {
		// unreachable: MkdirTemp fails only when TMPDIR itself is unwritable or full, a machine-level fault no test induces.
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "template")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		// unreachable: sql.Open only validates the driver name, which the sqlite3 import always registers.
		return nil, err
	}

	if _, err := conn.Exec(v9.ReferenceDDL); err != nil {
		// unreachable: ReferenceDDL is a fixed, valid schema every fixture test exercises; a syntax fault would fail the whole suite, not one path.
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Close(); err != nil {
		// unreachable: closing a connection with no pending writes and no locks held by another connection cannot fail.
		return nil, err
	}

	return os.ReadFile(path)
}

// writeReferenceSchema writes the reference schema template to path,
// replacing a fresh v9.ReferenceDDL exec against a new file with a byte
// copy of one built once for the whole test binary.
func writeReferenceSchema(tb testing.TB, path string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(path, referenceTemplate(tb), 0o600))
}
