package report_test

import (
	"context"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDiskRead = errors.New("read store status: disk read failed")

// fakeStore answers Status with a canned result or fault.
type fakeStore struct {
	status   store.Status
	accounts store.AccountList
	err      error
}

func (f fakeStore) Status(context.Context) (store.Status, error) { return f.status, f.err }

func (f fakeStore) Accounts(context.Context) (store.AccountList, error) { return f.accounts, f.err }

func Test_status_returns_what_the_store_reads(t *testing.T) {
	want := store.Status{Path: "/home/dave/quarry.duckdb", FormatVersion: 2, QuarryVersion: "v1.2.3"}
	srv := report.NewServer(report.WithStore(fakeStore{status: want}))

	got, err := srv.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func Test_status_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Status(t.Context())

	require.ErrorIs(t, err, errDiskRead)
}

func Test_home_returns_the_configured_home_directory(t *testing.T) {
	srv := report.NewServer(report.WithHome("/Users/dave"))

	assert.Equal(t, "/Users/dave", srv.Home())
}

func Test_SnapshotID(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "strips the directory and the sqlite extension", path: "/Users/dave/snapshots/20260927T143005Z.sqlite", want: "20260927T143005Z"},
		{name: "keeps a name that has no sqlite extension", path: "/snapshots/20260927T143005Z", want: "20260927T143005Z"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.SnapshotID(c.path))
		})
	}
}
