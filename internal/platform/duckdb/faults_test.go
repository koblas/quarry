package duckdb_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_IsDiskFull_and_IsPermission_classify_error_shapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		err          error
		wantDiskFull bool
		wantPerm     bool
	}{
		{
			name:     "os PathError permission denied",
			err:      &fs.PathError{Op: "open", Path: "x", Err: os.ErrPermission},
			wantPerm: true,
		},
		{
			name:         "os LinkError disk full",
			err:          &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.ENOSPC},
			wantDiskFull: true,
		},
		{
			name: "driver IO error, permission denied message",
			err: &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO,
				Msg: `IO Error: Cannot open file "x.duckdb": Permission denied`},
			wantPerm: true,
		},
		{
			name: "driver IO error, no space left message",
			err: &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO,
				Msg: `IO Error: Cannot write file "x.duckdb": No space left on device`},
			wantDiskFull: true,
		},
		{
			name: "driver IO error, disk quota exceeded message",
			err: &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO,
				Msg: `IO Error: Cannot write file "x.duckdb": ` + syscall.EDQUOT.Error()},
			wantDiskFull: true,
		},
		{
			name: "driver IO error, operation not permitted message",
			err: &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO,
				Msg: `IO Error: Cannot open file "x.duckdb": ` + syscall.EPERM.Error()},
			wantPerm: true,
		},
		{
			name: "driver IO error, unrelated message",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: file corrupt"},
		},
		{
			name: "driver error, non-IO type carrying disk-full text",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeCatalog, Msg: "no space left on device"},
		},
		{
			name: "non-duckdb error",
			err:  errors.New("boom"),
		},
		{
			name: "nil",
			err:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.wantDiskFull, duckdb.IsDiskFull(c.err))
			assert.Equal(t, c.wantPerm, duckdb.IsPermission(c.err))
		})
	}
}

// Real driver fault, not a constructed one: OpenReadOnly on a file the
// process cannot read at all.
func Test_open_read_only_on_a_permission_denied_file_classifies_as_permission(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := duckdb.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsPermission(err), "expected a permission-classified error, got %v", err)
}

// Control for the case above: the same file, readable, opens fine.
func Test_open_read_only_on_an_owner_readable_file_succeeds(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())
	require.NoError(t, os.Chmod(path, 0o400))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	db, err := duckdb.OpenReadOnly(t.Context(), path)

	require.NoError(t, err)
	_ = db.Close()
}

func Test_create_on_an_existing_path_reports_ErrExists_not_a_permission_fault(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	_, err := duckdb.Create(t.Context(), path)

	require.ErrorIs(t, err, duckdb.ErrExists)
	assert.False(t, duckdb.IsPermission(err))
}
