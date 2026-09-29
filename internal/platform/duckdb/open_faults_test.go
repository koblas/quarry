package duckdb_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DuckDB's own wording, capitals included: the predicates and the prefix strip match it exactly.
var (
	errOpenFaultTexts = errors.New("Could not set lock on file; not a valid DuckDB database file") //nolint:staticcheck // DuckDB's capitalised text
	errTwoLines       = errors.New("Parser Error: first\nsecond")                                  //nolint:staticcheck // DuckDB's capitalised type prefix
)

// lockHolderEnv names the database file Test_hold_a_database_open_for_writing holds when run as a child process.
const lockHolderEnv = "QUARRY_DUCKDB_LOCK_HOLDER"

func Test_open_read_only_on_a_non_duckdb_file_classifies_as_not_a_database(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content []byte
	}{
		{name: "a text file", content: []byte("not a database\n")},
		{name: "an empty file", content: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "data.duckdb")
			require.NoError(t, os.WriteFile(path, c.content, 0o600))

			_, err := duckdb.OpenReadOnly(t.Context(), path)

			require.Error(t, err)
			assert.True(t, duckdb.IsNotDatabase(err), err.Error())
		})
	}
}

// DuckDB's file lock is per process: only another process holding the file for writing can refuse the open.
func Test_open_read_only_on_a_file_another_process_holds_classifies_as_locked(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	holder := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^Test_hold_a_database_open_for_writing$") //nolint:gosec // re-executes this test binary with a fixed -test.run
	holder.Env = append(os.Environ(), lockHolderEnv+"="+path)
	release, err := holder.StdinPipe()
	require.NoError(t, err)
	out, err := holder.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, holder.Start())
	t.Cleanup(func() { _ = release.Close(); _ = holder.Wait() })
	ready, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "READY\n", ready)

	_, err = duckdb.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsLocked(err), err.Error())
}

// Child-process body for the locked test above; skipped when run directly.
func Test_hold_a_database_open_for_writing(t *testing.T) {
	path := os.Getenv(lockHolderEnv)
	if path == "" {
		t.Skip("runs only as the locked test's child process")
	}
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)

	_, _ = os.Stdout.WriteString("READY\n")
	_, _ = io.Copy(io.Discard, os.Stdin)

	require.NoError(t, db.Close())
}

func Test_open_fault_predicates_classify_driver_errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		err             error
		wantNotDatabase bool
		wantLocked      bool
	}{
		{
			name: "a file that is not a database",
			err: fmt.Errorf("open x read-only: %w", &duckdbdriver.Error{
				Type: duckdbdriver.ErrorTypeIO,
				Msg:  `IO Error: The file "x.duckdb" exists, but it is not a valid DuckDB database file!`,
			}),
			wantNotDatabase: true,
		},
		{
			name: "a file another process holds",
			err: fmt.Errorf("open x read-only: %w", &duckdbdriver.Error{
				Type: duckdbdriver.ErrorTypeIO,
				Msg:  `IO Error: Could not set lock on file "x.duckdb": Conflicting lock is held in /usr/local/bin/quarry (PID 42) by user dave. See also https://duckdb.org/docs/stable/connect/concurrency`,
			}),
			wantLocked: true,
		},
		{
			name: "lock text in a non-IO driver error",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeCatalog, Msg: "Could not set lock on file"},
		},
		{
			name: "not-a-database text in a non-IO driver error",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeCatalog, Msg: "not a valid DuckDB database file"},
		},
		{
			name: "an unrelated IO error",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: disk read failed"},
		},
		{
			name: "a non-driver error carrying the text",
			err:  errOpenFaultTexts,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.wantNotDatabase, duckdb.IsNotDatabase(c.err))
			assert.Equal(t, c.wantLocked, duckdb.IsLocked(c.err))
		})
	}
}

func Test_ErrorLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "the driver's message without its error type",
			err:  fmt.Errorf("open x read-only: %w", &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: disk read failed"}),
			want: "disk read failed",
		},
		{
			name: "only the first line of a multi-line driver message",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeCatalog, Msg: "Catalog Error: Table x does not exist!\nDid you mean y?"},
			want: "Table x does not exist!",
		},
		{
			name: "a driver message of only its error type is empty",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: "},
			want: "",
		},
		{
			name: "a type-like phrase after the start is kept",
			err:  &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "Cannot open: IO Error: x"},
			want: "Cannot open: IO Error: x",
		},
		{
			name: "the first line of an error with no driver error in its tree",
			err:  errTwoLines,
			want: "first",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, duckdb.ErrorLine(c.err))
		})
	}
}
