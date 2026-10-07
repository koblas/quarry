package snapshot_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// errNoSpace stands in for a stdout write failing on a full disk.
var errNoSpace = errors.New("no space left on device")

func Test_stdout_write_refusal_points_to_from_only_once_the_build_was_reached(t *testing.T) {
	t.Parallel()
	writeErr := errNoSpace
	manifest := snapshot.Manifest{Snapshot: snapshot.Info{
		Path: "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
	}}
	cases := []struct {
		name  string
		store *store.Result
		want  string
	}{
		{
			name:  "no build reached",
			store: nil,
			want: "cannot write the result to stdout: no space left on device; the snapshot is kept at " +
				"~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite and its .json manifest holds the full result",
		},
		{
			name:  "build reached",
			store: &store.Result{Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb"},
			want: "cannot write the result to stdout: no space left on device; " +
				"run quarry sync --from 20260927T143005Z --json to see it again",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outcome := snapshot.Outcome{Manifest: manifest, Store: c.store}

			got := outcome.StdoutWriteRefusal("/Users/dave", writeErr)

			assert.Equal(t, c.want, got.Error())
		})
	}
}

func Test_outcome_warnings_omit_one_sided_transfers_for_a_built_store(t *testing.T) {
	t.Parallel()
	built := &store.Result{Built: true, Validation: store.Validation{Transfers: store.TransferCheck{OneSided: make([]store.OneSidedTransfer, 2)}}}
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"schema warning"}}, Store: built}

	got := outcome.Warnings()

	assert.Equal(t, []string{"schema warning"}, got)
}

func Test_outcome_warnings_are_the_manifests_when_no_import_ran(t *testing.T) {
	t.Parallel()
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"schema warning"}}}

	got := outcome.Warnings()

	assert.Equal(t, []string{"schema warning"}, got)
}

func Test_id_strips_one_sqlite_extension_in_any_letter_case(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "lower case", path: "/snaps/20260927T143005Z.sqlite", want: "20260927T143005Z"},
		{name: "upper case", path: "/snaps/20260927T143005Z.SQLITE", want: "20260927T143005Z"},
		{name: "mixed case", path: "/snaps/20260927T143005Z.Sqlite", want: "20260927T143005Z"},
		{name: "the long s folds", path: "/snaps/20260927T143005Z.ſqlite", want: "20260927T143005Z"},
		{name: "another extension is kept", path: "/snaps/latest.db", want: "latest.db"},
		{name: "only the last extension goes", path: "/snaps/X.sqlite.SQLITE", want: "X.sqlite"},
		{name: "a manifest name is kept", path: "/snaps/X.json", want: "X.json"},
		{name: "no extension", path: "/snaps/X", want: "X"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, snapshot.ID(c.path))
		})
	}
}
