package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{name: "strips an upper-case extension", path: "/snapshots/20260927T143005Z.SQLITE", want: "20260927T143005Z"},
		{name: "strips a mixed-case extension from a name that is not an id", path: "/snapshots/latest.Sqlite", want: "latest"},
		{name: "keeps another extension", path: "/snapshots/20260927T143005Z.sqlite3", want: "20260927T143005Z.sqlite3"},
		{name: "strips one extension only", path: "/snapshots/a.sqlite.SQLITE", want: "a.sqlite"},
		{name: "ignores a directory named like the extension", path: "/a.SQLITE/20260927T143005Z", want: "20260927T143005Z"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.SnapshotID(c.path))
		})
	}
}
