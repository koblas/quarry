package store_test

import (
	"io/fs"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_open_error_names_the_store_and_its_fault(t *testing.T) {
	cases := []struct {
		name string
		err  *store.OpenError
		want string
	}{
		{name: "with an underlying fault", err: &store.OpenError{Path: "/s/quarry.duckdb", Err: fs.ErrNotExist}, want: "open store /s/quarry.duckdb: file does not exist"},
		{name: "without one", err: &store.OpenError{Path: "/s/quarry.duckdb"}, want: "open store /s/quarry.duckdb"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.EqualError(t, c.err, c.want)
		})
	}
}

func Test_open_error_unwraps_to_its_fault(t *testing.T) {
	err := &store.OpenError{Path: "/s/quarry.duckdb", Err: fs.ErrNotExist}

	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func Test_unreadable_reason_is_the_bare_phrase_for_each_fault(t *testing.T) {
	const at = "~/quarry.duckdb"
	cases := []struct {
		name string
		err  *store.OpenError
		want string
	}{
		{name: "not a DuckDB file", err: &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: "/s/quarry.duckdb"}, want: "the file is not a DuckDB database"},
		{name: "permission denied", err: &store.OpenError{Fault: store.OpenFaultPermission, Path: "/s/quarry.duckdb"}, want: "permission denied"},
		{name: "locked by another program", err: &store.OpenError{Fault: store.OpenFaultLocked, Path: "/s/quarry.duckdb"}, want: "another program has it open for writing"},
		{
			name: "another fault names the store by the display path",
			err:  &store.OpenError{Fault: store.OpenFaultOther, Path: "/s/quarry.duckdb", Reason: "cannot open /s/quarry.duckdb: broken"},
			want: "cannot open ~/quarry.duckdb: broken",
		},
		{
			name: "another fault with a ready phrase keeps it",
			err:  &store.OpenError{Fault: store.OpenFaultOther, Path: "/s/quarry.duckdb", Reason: "it has no import_runs table"},
			want: "it has no import_runs table",
		},
		{name: "another fault with no path keeps its reason whole", err: &store.OpenError{Fault: store.OpenFaultOther, Reason: "broken"}, want: "broken"},
		{name: "a missing store has no reason", err: &store.OpenError{Fault: store.OpenFaultMissing, Path: "/s/quarry.duckdb"}, want: ""},
		{name: "a store of another format has no reason", err: &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: "/s/quarry.duckdb"}, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.err.UnreadableReason(at))
		})
	}
}
