package snapshot_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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

func builtWithOneSided(n int) *store.Result {
	return &store.Result{Built: true, Validation: store.Validation{Transfers: store.TransferCheck{OneSided: make([]store.OneSidedTransfer, n)}}}
}

func Test_outcome_warnings_put_w1_before_w2(t *testing.T) {
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"W1 text"}}, Store: builtWithOneSided(2)}

	got := outcome.Warnings()

	assert.Equal(t, []string{
		"W1 text",
		"2 transfers have no matching transaction in another account; quarry keeps them as one-sided transfers",
	}, got)
}

func Test_outcome_warnings_names_a_single_one_sided_transfer_in_the_singular(t *testing.T) {
	outcome := snapshot.Outcome{Store: builtWithOneSided(1)}

	got := outcome.Warnings()

	assert.Equal(t, []string{
		"1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer",
	}, got)
}

func Test_outcome_warnings_omit_w2_when_every_transfer_is_paired(t *testing.T) {
	outcome := snapshot.Outcome{
		Manifest: snapshot.Manifest{Warnings: []string{"W1 text"}},
		Store:    &store.Result{Built: true, Validation: store.Validation{Transfers: store.TransferCheck{Paired: 3}}},
	}

	got := outcome.Warnings()

	assert.Equal(t, []string{"W1 text"}, got)
}

func Test_outcome_warnings_omit_w2_when_the_store_was_not_built(t *testing.T) {
	built, unbuilt := builtWithOneSided(2), builtWithOneSided(2)
	unbuilt.Built = false

	gotBuilt := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"W1 text"}}, Store: built}.Warnings()
	gotUnbuilt := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"W1 text"}}, Store: unbuilt}.Warnings()

	assert.Len(t, gotBuilt, 2)
	assert.Equal(t, []string{"W1 text"}, gotUnbuilt)
}

func Test_outcome_warnings_are_the_manifests_when_no_import_ran(t *testing.T) {
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"W1 text"}}}

	got := outcome.Warnings()

	assert.Equal(t, []string{"W1 text"}, got)
}
