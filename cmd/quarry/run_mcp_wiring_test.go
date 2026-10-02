package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newMCPServe_refuses_without_a_home_before_reporting_ready(t *testing.T) {
	t.Setenv("HOME", "")
	var ready bool

	err := newMCPServe(nil)(t.Context(), bytes.NewReader(nil), io.Discard, io.Discard, func() { ready = true })

	require.ErrorIs(t, err, errNoHome)
	require.EqualError(t, err, "cannot find your home directory ($HOME is not set); set HOME, then run quarry mcp again")
	assert.False(t, ready)
}

func Test_newMCPServe_reports_ready_once_it_is_serving(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var ready bool

	err := newMCPServe(nil)(t.Context(), bytes.NewReader(nil), io.Discard, io.Discard, func() { ready = true })

	require.NoError(t, err)
	assert.True(t, ready)
}

func Test_defaultEnv_probes_stdin_for_a_terminal(t *testing.T) {
	env := defaultEnv(io.Discard, io.Discard)

	assert.NotNil(t, env.IsTerminal)
}

func Test_isTerminal_reports_dev_null_as_no_terminal(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { _ = devNull.Close() })
	pipeRead, pipeWrite, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pipeRead.Close(); _ = pipeWrite.Close() })

	assert.False(t, isTerminal(devNull))
	assert.False(t, isTerminal(pipeRead))
	assert.False(t, isTerminal(&bytes.Buffer{}))
}
