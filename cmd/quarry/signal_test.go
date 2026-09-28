// signalContext is unexported, so its tests live in package main rather
// than importing main from outside.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("ctx.Done() did not close within 5s")
	}
}

func Test_signalContext_closes_ctx_done_on_sigint(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))

	waitDone(t, ctx)
}

func Test_signalContext_closes_ctx_done_on_sigterm(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	waitDone(t, ctx)
}

// signalSubprocessEnv, when set to "1" in the child's environment, tells
// Test_signalContext_kills_the_process_on_a_second_sigterm to run as the
// subprocess body instead of the parent's assertions.
const signalSubprocessEnv = "QUARRY_SIGNALCONTEXT_SUBPROCESS"

// Runs itself as a child process. SIGTERM, not SIGINT: a re-exec'd child may
// inherit SIGINT as ignored.
func Test_signalContext_kills_the_process_on_a_second_sigterm(t *testing.T) {
	if os.Getenv(signalSubprocessEnv) == "1" {
		runSignalSubprocessChild()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^Test_signalContext_kills_the_process_on_a_second_sigterm$")
	cmd.Env = append(os.Environ(), signalSubprocessEnv+"=1")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	reader := bufio.NewReader(stdout)
	require.Equal(t, "ready\n", readLineWithDeadline(t, reader, 5*time.Second))

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.Equal(t, "done\n", readLineWithDeadline(t, reader, 5*time.Second))

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-waitErr:
			var exitErr *exec.ExitError
			require.ErrorAs(t, result, &exitErr)
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			require.True(t, ok)
			assert.True(t, ws.Signaled(), "expected the child to die by signal, got %v", exitErr.ProcessState)
			assert.Equal(t, syscall.SIGTERM, ws.Signal())
			return
		case <-ticker.C:
			_ = cmd.Process.Signal(syscall.SIGTERM)
		case <-deadline:
			t.Fatal("child did not exit after repeated SIGTERM; the second signal was swallowed")
		}
	}
}

// runSignalSubprocessChild is the subprocess body for the second-SIGTERM
// test: it never returns, only dies to a signal once the parent stops
// diverting it (or hangs, on the mutation the test exists to catch).
func runSignalSubprocessChild() {
	ctx, stop := signalContext(context.Background())
	defer stop()
	fmt.Println("ready")
	<-ctx.Done()
	fmt.Println("done")
	time.Sleep(30 * time.Second)
}

// readLineWithDeadline reads one newline-terminated line from r, failing t
// if none arrives within timeout.
func readLineWithDeadline(t *testing.T, r *bufio.Reader, timeout time.Duration) string {
	t.Helper()
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := r.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		return line
	case err := <-errCh:
		t.Fatalf("reading child output: %v", err)
	case <-time.After(timeout):
		t.Fatal("timed out waiting for child output")
	}
	return ""
}
