package snapshot_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// Before the store was built, the refusal points at the manifest already on
// disk; once it was built, the manifest alone no longer carries the store
// result, so the refusal points at --from --json instead.
func Test_stdout_write_refusal_points_to_from_only_once_the_build_was_reached(t *testing.T) {
	writeErr := errors.New("no space left on device")
	manifest := snapshot.Manifest{Snapshot: snapshot.SnapshotInfo{
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
			outcome := snapshot.Outcome{Manifest: manifest, Store: c.store}

			got := outcome.StdoutWriteRefusal("/Users/dave", writeErr)

			assert.Equal(t, c.want, got.Error())
		})
	}
}
