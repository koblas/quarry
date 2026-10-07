package toolrun_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/toolrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The helper commands re-run this test binary, so no test starts a real tool.
const (
	helperInterleave = "helper-interleave"
	helperBoth       = "helper-both"
	helperExit       = "helper-exit"
	helperFail       = "helper-fail"
	helperStdin      = "helper-stdin"
	helperHang       = "helper-hang"
	helperSelfKill   = "helper-self-kill"
	helperParent     = "helper-parent"
	helperGrandchild = "helper-grandchild"
	helperLifetime   = time.Minute

	// interleaveGap separates writes to different pipes far enough that their arrival order is fixed.
	interleaveGap = 50 * time.Millisecond
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "helper-") {
		os.Exit(helperMain(os.Args[1], os.Args[2:]))
	}
	os.Exit(m.Run())
}

// helperMain runs the helper command named by the first argument.
func helperMain(name string, args []string) int {
	switch name {
	case helperInterleave:
		_, _ = os.Stdout.WriteString("out1\n")
		time.Sleep(interleaveGap)
		_, _ = os.Stderr.WriteString("err1\n")
		time.Sleep(interleaveGap)
		_, _ = os.Stdout.WriteString("out2\n")
		return 0
	case helperBoth:
		_, _ = os.Stderr.WriteString("err\n")
		_, _ = os.Stdout.WriteString("out\n")
		return 0
	case helperExit:
		code, _ := strconv.Atoi(args[0])
		return code
	case helperFail:
		_, _ = os.Stderr.WriteString("err\n")
		time.Sleep(interleaveGap)
		_, _ = os.Stdout.WriteString("out\n")
		code, _ := strconv.Atoi(args[0])
		return code
	case helperStdin:
		return stdinIsNullDevice()
	case helperHang:
		writeErrThenOut()
		//nolint:gosec // the ready file is a tempdir path the test passed
		if os.WriteFile(args[0], nil, 0o600) != nil {
			return 4
		}
		time.Sleep(helperLifetime)
		return 0
	case helperSelfKill:
		writeErrThenOut()
		self, err := os.FindProcess(os.Getpid())
		if err != nil || self.Kill() != nil {
			return 4
		}
		time.Sleep(helperLifetime)
		return 0
	case helperParent:
		return spawnGrandchild()
	case helperGrandchild:
		time.Sleep(helperLifetime)
		return 0
	}
	return 99
}

// writeErrThenOut writes "err" to stderr, then "out" to stdout after interleaveGap.
func writeErrThenOut() {
	_, _ = os.Stderr.WriteString("err\n")
	time.Sleep(interleaveGap)
	_, _ = os.Stdout.WriteString("out\n")
}

// stdinIsNullDevice exits 0 when stdin is the null device and 7 otherwise.
func stdinIsNullDevice() int {
	in, err := os.Stdin.Stat()
	if err != nil {
		return 6
	}
	null, err := os.Stat(os.DevNull)
	if err != nil || !os.SameFile(in, null) {
		return 7
	}
	return 0
}

// spawnGrandchild starts a long-lived child that shares this process's output,
// prints its pid, and exits without waiting for it.
func spawnGrandchild() int {
	child := execSelf(helperGrandchild)
	if child.Start() != nil {
		return 5
	}
	_, _ = os.Stdout.WriteString(strconv.Itoa(child.Process.Pid))
	return 0
}

func Test_run_combines_stdout_and_stderr_in_read_order_when_writes_are_spaced_apart(t *testing.T) {
	_, combined, _, err := toolrun.Run(t.Context(), os.Args[0], helperInterleave)

	require.NoError(t, err)
	assert.Equal(t, "out1\nerr1\nout2\n", string(combined))
}

func Test_run_returns_stdout_without_stderr(t *testing.T) {
	stdout, _, _, err := toolrun.Run(t.Context(), os.Args[0], helperBoth)

	require.NoError(t, err)
	assert.Equal(t, "out\n", string(stdout))
}

func Test_run_returns_the_output_of_a_command_that_exits_non_zero(t *testing.T) {
	stdout, combined, status, err := toolrun.Run(t.Context(), os.Args[0], helperFail, "3")

	require.NoError(t, err)
	assert.Equal(t, 3, status)
	assert.Equal(t, "err\nout\n", string(combined))
	assert.Equal(t, "out\n", string(stdout))
}

func Test_run_returns_the_exit_status_with_no_error(t *testing.T) {
	for _, status := range []int{0, 3} {
		t.Run("status "+strconv.Itoa(status), func(t *testing.T) {
			_, _, got, err := toolrun.Run(t.Context(), os.Args[0], helperExit, strconv.Itoa(status))

			require.NoError(t, err)
			assert.Equal(t, status, got)
		})
	}
}

// Swaps os.Stdin for a pipe that never delivers data, so inheriting it is visible.
func Test_run_gives_the_child_an_empty_stdin(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
		_ = writer.Close()
	})

	_, _, status, err := toolrun.Run(t.Context(), os.Args[0], helperStdin)

	require.NoError(t, err)
	assert.Equal(t, 0, status)
}

var errExecFormat = errors.New("exec format error")

func Test_start_error_names_the_file_and_the_cause(t *testing.T) {
	err := &toolrun.StartError{Path: "/opt/bin/claude", Err: errExecFormat}

	assert.Equal(t, "cannot start /opt/bin/claude: exec format error", err.Error())
}

func Test_signal_error_names_the_signal(t *testing.T) {
	err := &toolrun.SignalError{Signal: syscall.SIGKILL}

	assert.Equal(t, "stopped by signal: killed", err.Error())
}

func Test_run_reports_a_file_it_cannot_start(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(notExecutable, []byte("not a program"), 0o600))
	missing := filepath.Join(t.TempDir(), "claude")
	cases := []struct {
		name string
		path string
		want syscall.Errno
	}{
		{"a file without the execute bit", notExecutable, syscall.EACCES},
		{"a path that does not exist", missing, syscall.ENOENT},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, combined, status, err := toolrun.Run(t.Context(), c.path)

			var start *toolrun.StartError
			require.ErrorAs(t, err, &start)
			assert.Equal(t, c.path, start.Path)
			var pathErr *fs.PathError
			require.ErrorAs(t, err, &pathErr)
			assert.Equal(t, c.want, pathErr.Err)
			assert.Equal(t, -1, status)
			assert.Empty(t, stdout)
			assert.Empty(t, combined)
		})
	}
}

// cancelled is what Run returned for a command cancelled mid-run.
type cancelled struct {
	stdout   []byte
	combined []byte
	status   int
	err      error
}

// cancelHungCommand cancels the context of a command that has printed and is waiting.
func cancelHungCommand(t *testing.T) cancelled {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan cancelled, 1)
	go func() {
		stdout, combined, status, err := toolrun.Run(ctx, os.Args[0], helperHang, ready)
		done <- cancelled{stdout, combined, status, err}
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(ready); return err == nil }, 10*time.Second, 5*time.Millisecond)

	cancel()
	return <-done
}

func Test_run_returns_the_context_error_when_cancelled_mid_run(t *testing.T) {
	got := cancelHungCommand(t)

	require.ErrorIs(t, got.err, context.Canceled)
	assert.NotErrorAs(t, got.err, new(*toolrun.SignalError))
	assert.Equal(t, -1, got.status)
}

func Test_run_returns_the_output_of_a_command_cancelled_mid_run(t *testing.T) {
	got := cancelHungCommand(t)

	assert.Equal(t, "out\n", string(got.stdout))
	assert.Equal(t, "err\nout\n", string(got.combined))
}

func Test_run_returns_the_context_error_when_cancelled_before_start(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	missing := filepath.Join(t.TempDir(), "claude")

	_, _, status, err := toolrun.Run(ctx, missing)

	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorAs(t, err, new(*toolrun.StartError))
	assert.Equal(t, -1, status)
}

func Test_run_reports_a_signal_it_did_not_send(t *testing.T) {
	_, _, status, err := toolrun.Run(t.Context(), os.Args[0], helperSelfKill)

	var signalled *toolrun.SignalError
	require.ErrorAs(t, err, &signalled)
	assert.Equal(t, "killed", signalled.Signal.String())
	assert.Equal(t, -1, status)
}

func Test_run_returns_the_output_of_a_command_ended_by_a_signal(t *testing.T) {
	stdout, combined, _, err := toolrun.Run(t.Context(), os.Args[0], helperSelfKill)

	require.ErrorAs(t, err, new(*toolrun.SignalError))
	assert.Equal(t, "out\n", string(stdout))
	assert.Equal(t, "err\nout\n", string(combined))
}

func Test_run_returns_when_a_grandchild_holds_the_output_open(t *testing.T) {
	const waitDelay = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	began := time.Now()

	_, out, status, err := toolrun.RunWithWaitDelay(ctx, waitDelay, os.Args[0], helperParent)

	elapsed := time.Since(began)
	pid, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, convErr)
	t.Cleanup(func() { killProcess(pid) })
	require.NoError(t, err)
	assert.Equal(t, 0, status)
	assert.Less(t, elapsed, 10*time.Second)
}

// The shipped wait delay is what bounds Run, so this goes through Run itself.
func Test_run_returns_at_the_shipped_wait_delay_when_a_grandchild_holds_the_output_open(t *testing.T) {
	type outcome struct {
		out    []byte
		status int
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		_, out, status, err := toolrun.Run(t.Context(), os.Args[0], helperParent)
		done <- outcome{out, status, err}
	}()

	select {
	case got := <-done:
		pid, convErr := strconv.Atoi(strings.TrimSpace(string(got.out)))
		require.NoError(t, convErr)
		t.Cleanup(func() { killProcess(pid) })
		require.NoError(t, got.err)
		assert.Equal(t, 0, got.status)
	case <-time.After(10 * time.Second):
		require.Fail(t, "Run did not return while a grandchild held the output open")
	}
}
