package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpPingRequest = `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"

// errBrokenPipe is the write error a closed stdout pipe produces.
var errBrokenPipe = &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}

// capturedStdout records what is written to it, or fails every write with err when err is set.
type capturedStdout struct {
	written bytes.Buffer
	err     error
}

func (c *capturedStdout) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	return c.written.Write(p)
}

func Test_run_mcp_ends_with_the_ruled_exit_code(t *testing.T) {
	const home = "/home/quarry-test"
	cases := []struct {
		name       string
		args       []string
		home       string
		stdin      func(t *testing.T) io.Reader
		stdoutErr  error
		terminal   bool
		cancelled  bool
		wantCode   int
		wantStderr string
	}{
		{
			name: "the client closes stdin", args: []string{"mcp"}, home: home,
			stdin:    func(*testing.T) io.Reader { return strings.NewReader("") },
			wantCode: 0,
		},
		{
			name: "the process is told to stop", args: []string{"mcp"}, home: home,
			stdin:     openPipeCarrying(""),
			cancelled: true,
			wantCode:  0,
		},
		{
			name: "the client stops reading stdout", args: []string{"mcp"}, home: home,
			stdin:     openPipeCarrying(mcpPingRequest),
			stdoutErr: errBrokenPipe,
			wantCode:  0,
		},
		{
			name: "an argument is given", args: []string{"mcp", "extra"}, home: home,
			stdin:      func(*testing.T) io.Reader { return strings.NewReader("") },
			wantCode:   2,
			wantStderr: "quarry: mcp takes no arguments\n",
		},
		{
			name: "--json is given", args: []string{"mcp", "--json"}, home: home,
			stdin:      func(*testing.T) io.Reader { return strings.NewReader("") },
			wantCode:   2,
			wantStderr: "quarry: mcp always speaks JSON on stdout; drop --json\n",
		},
		{
			name: "$HOME is unset", args: []string{"mcp"}, home: "",
			stdin:      func(*testing.T) io.Reader { return strings.NewReader("") },
			wantCode:   1,
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry mcp again\n",
		},
		{
			name: "$HOME is unset at a terminal", args: []string{"mcp"}, home: "", terminal: true,
			stdin:      func(*testing.T) io.Reader { return strings.NewReader("") },
			wantCode:   1,
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry mcp again\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", c.home)
			stdout := &capturedStdout{err: c.stdoutErr}
			var stderr bytes.Buffer
			env := testEnv(stdout, &stderr)
			env.Stdin = c.stdin(t)
			env.IsTerminal = func(io.Reader) bool { return c.terminal }

			code := runWith(deadlineContext(t, c.cancelled), c.args, env)

			assert.Equal(t, c.wantCode, code)
			assert.Equal(t, c.wantStderr, stderr.String())
			assert.Empty(t, stdout.written.String())
		})
	}
}

// openPipeCarrying returns a stdin that delivers first and then stays open until the test ends.
func openPipeCarrying(first string) func(t *testing.T) io.Reader {
	return func(t *testing.T) io.Reader {
		t.Helper()
		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })
		go func() {
			if first != "" {
				_, _ = io.WriteString(writer, first)
			}
		}()
		return reader
	}
}

// deadlineContext is a context that ends at mcpTestDeadline, or is already cancelled when cancelled is set.
func deadlineContext(t *testing.T, cancelled bool) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	t.Cleanup(cancel)
	if cancelled {
		cancel()
	}
	return ctx
}

// mcpPipeSubprocessEnv set to "1" makes the re-executed test binary run as quarry mcp.
const mcpPipeSubprocessEnv = "QUARRY_MCP_PIPE_SUBPROCESS"

// Re-executes the test binary: only a real process dies of SIGPIPE on a broken fd 1.
func Test_quarry_mcp_exits_0_when_the_real_stdout_pipe_breaks(t *testing.T) {
	if os.Getenv(mcpPipeSubprocessEnv) == "1" {
		os.Exit(runProcess(t.Context(), []string{"mcp"}, os.Stdout, os.Stderr))
	}
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], //nolint:gosec // re-executes this test binary with a fixed -test.run
		"-test.run=^Test_quarry_mcp_exits_0_when_the_real_stdout_pipe_breaks$")
	cmd.Env = append(os.Environ(), mcpPipeSubprocessEnv+"=1", "HOME="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	require.NoError(t, stdout.Close())
	_, writeErr := io.WriteString(stdin, mcpPingRequest)
	waitErr := cmd.Wait()

	require.NoError(t, writeErr)
	assert.NoError(t, waitErr)
	assert.Empty(t, stderr.String())
}
