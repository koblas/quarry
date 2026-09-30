// exitCode is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
)

func Test_exitCode_is_1_and_prints_nothing_for_an_error_already_reported(t *testing.T) {
	var stderr bytes.Buffer

	code := exitCode(cli.ReportedError{}, &stderr)

	assert.Equal(t, 1, code)
	assert.Empty(t, stderr.String())
}

var errBoom = errors.New("boom")

func Test_exitCode_prints_an_error_that_was_not_reported_and_exits_1(t *testing.T) {
	var stderr bytes.Buffer

	code := exitCode(errBoom, &stderr)

	assert.Equal(t, 1, code)
	assert.Equal(t, "quarry: boom\n", stderr.String())
}
