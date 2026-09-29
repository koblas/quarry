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
