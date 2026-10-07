package toolrun

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// waitDelay bounds how long Run waits for a command's output after the command
// itself has exited, so a grandchild that inherited the output cannot hang Run.
const waitDelay = 2 * time.Second

// noStatus is the exit status returned alongside any error.
const noStatus = -1

// StartError reports a command that could not be started, such as a file that
// is not executable or has vanished.
type StartError struct {
	Path string // the file Run tried to start
	Err  error  // the start failure, typically a *fs.PathError
}

func (e *StartError) Error() string { return "cannot start " + e.Path + ": " + e.Err.Error() }

func (e *StartError) Unwrap() error { return e.Err }

// SignalError reports a command ended by a signal that Run did not send.
type SignalError struct {
	Signal os.Signal // the signal that ended the command
}

func (e *SignalError) Error() string { return "stopped by signal: " + e.Signal.String() }

// Run runs the command name with args and an empty stdin, and returns its stdout,
// its stdout and stderr combined in the order they were read, and its exit status; a non-zero
// status comes back with a nil error. err is ctx.Err(), *StartError or *SignalError, with status -1.
func Run(ctx context.Context, name string, args ...string) ([]byte, []byte, int, error) {
	return run(ctx, waitDelay, name, args...)
}

// syncBuffer is a bytes.Buffer safe for the two copying goroutines exec starts
// when a command's stdout and stderr are different writers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // bytes.Buffer.Write never fails
}

// Bytes returns a copy of what has been written.
func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// run is Run with the wait delay as a parameter, so a test can shorten it.
func run(ctx context.Context, delay time.Duration, name string, args ...string) ([]byte, []byte, int, error) {
	var stdout, output syncBuffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.MultiWriter(&stdout, &output)
	cmd.Stderr = &output
	cmd.WaitDelay = delay

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return nil, nil, noStatus, ctx.Err() //nolint:wrapcheck // the context's own error is the contract
		}
		return nil, nil, noStatus, &StartError{Path: name, Err: err}
	}
	err := cmd.Wait()
	switch {
	case err == nil:
		return stdout.Bytes(), output.Bytes(), 0, nil
	case ctx.Err() != nil:
		// CommandContext kills with SIGKILL when ctx ends, so ctx is decided
		// before the process state, which would read as a signal Run did not send.
		return stdout.Bytes(), output.Bytes(), noStatus, ctx.Err()
	}
	// ExitCode is -1 for a signalled process and for a nil ProcessState.
	if code := cmd.ProcessState.ExitCode(); code >= 0 {
		return stdout.Bytes(), output.Bytes(), code, nil
	}
	if state := cmd.ProcessState; state != nil {
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return stdout.Bytes(), output.Bytes(), noStatus, &SignalError{Signal: status.Signal()}
		}
	}
	// unreachable: on unix ExitCode() < 0 means signalled, returned above; the only other way here
	// is a nil ProcessState (wait4 failing with ECHILD), which no test can provoke.
	return stdout.Bytes(), output.Bytes(), noStatus, err
}
