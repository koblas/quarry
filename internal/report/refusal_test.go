package report_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	refusalHome = "/Users/dave"
	storePath   = "/Users/dave/Library/Application Support/quarry/quarry.duckdb"
)

func Test_status_refuses_with_the_store_refusal_copy(t *testing.T) {
	cases := []struct {
		name    string
		openErr *store.OpenError
		want    string
	}{
		{
			name:    "no store",
			openErr: &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath, Err: fs.ErrNotExist},
			want:    "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it",
		},
		{
			name: "another format, naming its snapshot",
			openErr: &store.OpenError{
				Fault: store.OpenFaultOtherFormat, Path: storePath,
				SnapshotPath: "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
			},
			want: "the store at ~/Library/Application Support/quarry/quarry.duckdb was built by another version of quarry; " +
				"run quarry sync --from 20260927T143005Z to rebuild it",
		},
		{
			name:    "another format with no readable snapshot",
			openErr: &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: storePath},
			want: "the store at ~/Library/Application Support/quarry/quarry.duckdb was built by another version of quarry; " +
				"run quarry sync to rebuild it",
		},
		{
			name:    "not a DuckDB file",
			openErr: &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath},
			want: "cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: " +
				"the file is not a DuckDB database; run quarry sync to rebuild it",
		},
		{
			name:    "permission",
			openErr: &store.OpenError{Fault: store.OpenFaultPermission, Path: storePath},
			want:    "cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: permission denied; run quarry sync to rebuild it",
		},
		{
			name:    "locked",
			openErr: &store.OpenError{Fault: store.OpenFaultLocked, Path: storePath},
			want: "cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: " +
				"another program has it open for writing; close that program and run the command again",
		},
		{
			name: "another fault, its reason naming the store",
			openErr: &store.OpenError{
				Fault: store.OpenFaultOther, Path: storePath, Reason: `Could not read from file "` + storePath + `": Is a directory`,
			},
			want: `cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: ` +
				`Could not read from file "~/Library/Application Support/quarry/quarry.duckdb": Is a directory; run quarry sync to rebuild it`,
		},
		{
			name:    "a store outside home",
			openErr: &store.OpenError{Fault: store.OpenFaultMissing, Path: "/srv/quarry/quarry.duckdb"},
			want:    "no store at /srv/quarry/quarry.duckdb yet; run quarry sync to build it",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{err: c.openErr}), report.WithHome(refusalHome))

			_, err := srv.Status(t.Context())

			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_accounts_refuses_with_the_store_refusal_copy(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultPermission, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Accounts(t.Context(), false)

	assert.EqualError(t, err,
		"cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: permission denied; run quarry sync to rebuild it")
}

func Test_spend_refuses_with_the_store_refusal_copy(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultPermission, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Spend(t.Context(), report.SpendRequest{Now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)})

	assert.EqualError(t, err,
		"cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: permission denied; run quarry sync to rebuild it")
}

func Test_query_refuses_with_the_store_refusal_copy(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	var gotMaxRows int
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr, gotMaxRows: &gotMaxRows}), report.WithHome(refusalHome))

	_, err := srv.Query(t.Context(), "SELECT 1", 500)

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_a_store_refusal_unwraps_to_the_store_error(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Status(t.Context())

	require.ErrorIs(t, err, openErr)
}

func Test_query_keeps_an_open_interrupted_by_its_context_as_interrupted(t *testing.T) {
	interrupted := store.Interrupted(&store.OpenError{Fault: store.OpenFaultMissing, Path: storePath})
	var gotMaxRows int
	srv := report.NewServer(report.WithStore(fakeStore{err: interrupted, gotMaxRows: &gotMaxRows}), report.WithHome(refusalHome))

	_, err := srv.Query(t.Context(), "SELECT 1", 500)

	assert.Equal(t, interrupted, err)
}

func Test_reads_report_an_interrupt_before_any_store_refusal(t *testing.T) {
	cases := []struct {
		name string
		read func(ctx context.Context, srv *report.Server) error
		want string
	}{
		{
			name: "status",
			read: func(ctx context.Context, srv *report.Server) error { _, err := srv.Status(ctx); return err },
			want: "status interrupted",
		},
		{
			name: "accounts",
			read: func(ctx context.Context, srv *report.Server) error { _, err := srv.Accounts(ctx, false); return err },
			want: "accounts interrupted",
		},
		{
			name: "spend",
			read: func(ctx context.Context, srv *report.Server) error {
				_, err := srv.Spend(ctx, report.SpendRequest{Now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)})
				return err
			},
			want: "spend interrupted",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
			srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := c.read(ctx, srv)

			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_reads_return_a_fault_that_is_not_a_store_refusal_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}), report.WithHome(refusalHome))

	_, err := srv.Status(t.Context())

	assert.Equal(t, errDiskRead, err)
}
