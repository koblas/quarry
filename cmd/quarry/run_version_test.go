package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_version_refuses_arguments(t *testing.T) {
	exitCode, stdout, stderr := runCapture(t.Context(), []string{"version", "foo"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: version takes no arguments\n", stderr.String())
}
