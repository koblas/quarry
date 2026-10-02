package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
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
