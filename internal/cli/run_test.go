package cli_test

import (
	"bytes"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
)

// Cobra falls back to reading os.Args when SetArgs is given a nil slice, so
// Execute must normalize nil itself rather than let cobra reach for the test
// binary's own arguments.
func Test_Execute_treats_nil_args_the_same_as_an_explicit_empty_slice(t *testing.T) {
	srv := snapshot.NewServer()
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), nil, &stdout, &stderr, srv, "/Users/dave")

	assert.NoError(t, err)
}
