package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errTwoLines = errors.New("connector refused\nsecond line")
	errEmpty    = errors.New("")
)

// snapshotFile is the snapshot path fixtures record in import_runs.
const snapshotFile = "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite"

// importRunsDDL is an import run naming snapshotFile, as every quarry store has.
const importRunsDDL = `CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);
INSERT INTO import_runs VALUES (1, '` + snapshotFile + `');`

// storeInfoDDL creates store_info holding one row per format version given.
func storeInfoDDL(versions ...string) string {
	var ddl strings.Builder
	ddl.WriteString("CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);")
	for _, v := range versions {
		ddl.WriteString("INSERT INTO store_info VALUES (" + v + ", '(devel)', now());")
	}
	return ddl.String()
}

// newStoreFile builds a DuckDB file at a Store's path from ddl, closed before any read.
func newStoreFile(t *testing.T, ddl string, opts ...duckstore.Option) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := duckdb.Create(t.Context(), filepath.Join(dir, duckstore.FileName))
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), ddl)
	require.NoError(t, err)
	require.NoError(t, db.CheckpointClose(t.Context()))
	return duckstore.New(dir, opts...)
}

// openRefusal is the *store.OpenError a read of st refuses with.
func openRefusal(t *testing.T, st *duckstore.Store) *store.OpenError {
	t.Helper()
	_, err := st.Status(t.Context())
	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	return openErr
}

// driverIOError is the error chain duckdb.OpenReadOnly returns for a driver IO fault reading msg.
func driverIOError(msg string) error {
	return fmt.Errorf("open x read-only: %w", &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: msg})
}

func Test_open_read_classifies_each_store_file_it_cannot_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(t *testing.T, dir string)
		want    store.OpenFault
	}{
		{name: "no file", arrange: func(*testing.T, string) {}, want: store.OpenFaultMissing},
		{
			name: "a file that is not a database",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
			},
			want: store.OpenFaultNotDuckDB,
		},
		{
			name: "a file the process may not read",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				skipAsRoot(t)
				path := filepath.Join(dir, duckstore.FileName)
				require.NoError(t, os.WriteFile(path, []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))
			},
			want: store.OpenFaultPermission,
		},
		{
			name: "a directory the process may not search",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(dir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			},
			want: store.OpenFaultPermission,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			c.arrange(t, dir)

			got := openRefusal(t, duckstore.New(dir))

			assert.Equal(t, c.want, got.Fault)
		})
	}
}

func Test_open_read_classifies_each_fault_the_open_returns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
		want  store.OpenFault
	}{
		{
			name: "another process holds the file",
			fault: driverIOError(`IO Error: Could not set lock on file "x.duckdb": Conflicting lock is held in ` +
				`/usr/local/bin/quarry (PID 42) by user dave. See also https://duckdb.org/docs/stable/connect/concurrency`),
			want: store.OpenFaultLocked,
		},
		{
			name:  "an OS permission fault",
			fault: &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission},
			want:  store.OpenFaultPermission,
		},
		{
			name:  "the driver's missing-database text while the file is there",
			fault: driverIOError(`IO Error: Cannot open database "x.duckdb" in read-only mode: database does not exist`),
			want:  store.OpenFaultOther,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, failingOpener(c.fault))

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.Fault)
		})
	}
}

func Test_open_read_refuses_a_store_removed_between_the_check_and_the_open(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, duckstore.WithOpenReadOnly(func(_ context.Context, path string) (duckstore.ReadDB, error) {
		require.NoError(t, os.Remove(path))
		return nil, driverIOError(`IO Error: Cannot open database "x.duckdb" in read-only mode: database does not exist`)
	}))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultMissing, got.Fault)
}

func Test_open_read_refuses_a_store_of_another_format(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
	}{
		{name: "no store_info", ddl: importRunsDDL},
		{name: "an older format", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion-1))},
		{name: "a newer format", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion+1))},
		{name: "a NULL format", ddl: importRunsDDL + storeInfoDDL("NULL")},
		{name: "store_info with no row", ddl: importRunsDDL + storeInfoDDL()},
		{
			name: "store_info with two rows of this format",
			ddl:  importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion), strconv.Itoa(duckstore.FormatVersion)),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl)

			got := openRefusal(t, st)

			assert.Equal(t, store.OpenError{
				Fault: store.OpenFaultOtherFormat, Path: st.Path(), SnapshotPath: snapshotFile, Err: got.Err,
			}, *got)
		})
	}
}

func Test_open_read_names_a_snapshot_only_when_import_runs_yields_one(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
		want string
	}{
		{name: "one import run", ddl: importRunsDDL, want: snapshotFile},
		{name: "no import_runs table", ddl: "CREATE TABLE accounts (id VARCHAR);", want: ""},
		{name: "no snapshot_path column", ddl: "CREATE TABLE import_runs (id BIGINT); INSERT INTO import_runs VALUES (1);", want: ""},
		{name: "no import run", ddl: "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);", want: ""},
		{name: "a NULL snapshot path", ddl: "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR); INSERT INTO import_runs VALUES (1, NULL);", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl)

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.SnapshotPath)
		})
	}
}

func Test_open_read_names_the_latest_import_runs_snapshot(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsDDL+
		"INSERT INTO import_runs VALUES (3, '/snapshots/run-3.sqlite'); INSERT INTO import_runs VALUES (2, '/snapshots/run-2.sqlite');")

	got := openRefusal(t, st)

	assert.Equal(t, "/snapshots/run-3.sqlite", got.SnapshotPath)
}

// The store's directory is reached through a symlink, so the path given and the path the driver names differ on every OS.
func Test_open_read_reports_other_faults_naming_the_store_by_its_given_path(t *testing.T) {
	t.Parallel()
	realDir := t.TempDir()
	_, err := duckstore.New(realDir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, linkDir))
	given := filepath.Join(linkDir, duckstore.FileName)
	resolved, err := filepath.EvalSymlinks(given)
	require.NoError(t, err)
	st := duckstore.New(linkDir, failingOpener(driverIOError(`IO Error: Cannot open file "`+resolved+`": Input/output error`)))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultOther, got.Fault)
	assert.Equal(t, `Cannot open file "`+given+`": Input/output error`, got.Reason)
}

func Test_open_read_reports_other_faults_with_a_one_line_reason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
		want  string
	}{
		{
			name:  "the driver's first line without its error type",
			fault: driverIOError("IO Error: Could not read from file: Is a directory\nmore detail"),
			want:  "Could not read from file: Is a directory",
		},
		{
			name:  "only the OS reason of a path error",
			fault: &fs.PathError{Op: "open", Path: "/elsewhere/quarry.duckdb", Err: syscall.EIO},
			want:  syscall.EIO.Error(),
		},
		{name: "the first line of any other error", fault: errTwoLines, want: "connector refused"},
		{name: "a driver message of only its error type", fault: driverIOError("IO Error: "), want: "unknown error"},
		{name: "an empty error", fault: errEmpty, want: "unknown error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, failingOpener(c.fault))

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.Reason)
		})
	}
}

func Test_open_read_reports_a_failed_format_check_as_another_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
	}{
		{name: "the catalog query", query: duckstore.ColumnExistsQuery},
		{name: "the format_version read", query: duckstore.FormatVersionQuery},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{checkFaults: map[string]error{c.query: fault}}))

			got := openRefusal(t, st)

			assert.Equal(t, store.OpenFaultOther, got.Fault)
			assert.ErrorIs(t, got, fault)
		})
	}
}

func Test_open_read_refuses_another_format_without_a_snapshot_when_its_read_fails(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{checkFaults: map[string]error{duckstore.SnapshotPathQuery: ioFault(`query rows "SELECT"`)}}
	st := newStoreFile(t, importRunsDDL, spyOpener(spy))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultOtherFormat, got.Fault)
	assert.Empty(t, got.SnapshotPath)
}

func Test_open_read_closes_the_connection_when_it_refuses_the_store(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ddl   string
		spy   *spyReadDB
		fault store.OpenFault
	}{
		{name: "another format", ddl: importRunsDDL, spy: &spyReadDB{}, fault: store.OpenFaultOtherFormat},
		{
			name: "a failed format check", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion)),
			spy:   &spyReadDB{checkFaults: map[string]error{duckstore.FormatVersionQuery: errQueryFailed}},
			fault: store.OpenFaultOther,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl, spyOpener(c.spy))

			got := openRefusal(t, st)

			require.Equal(t, c.fault, got.Fault)
			assert.Equal(t, 1, c.spy.closes)
		})
	}
}
