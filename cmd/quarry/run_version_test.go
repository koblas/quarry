package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_version_refuses_arguments(t *testing.T) {
	exitCode, stdout, stderr := runCapture(t.Context(), []string{"version", "foo"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: version takes no arguments\n", stderr.String())
}

func Test_run_version_prints_without_a_home(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(t.Context(), []string{"version"})

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "quarry (devel)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_defaultEnv_carries_this_binarys_build_info(t *testing.T) {
	env := defaultEnv(io.Discard, io.Discard)

	assert.NotNil(t, env.BuildInfo)
}
