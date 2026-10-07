// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What `go test` binaries report as their main module version.
const mcpTestServerVersion = "(devel)"

func Test_run_mcp_lists_quarrys_tools_over_json_rpc(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	session := peer.session

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, session.Close())
	peer.waitForExit(ctx, t)
	stdout, stderr := peer.stdout, peer.stderr

	assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: mcpTestServerVersion}, session.InitializeResult().ServerInfo)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"query", "describe_schema", "sync_status", "data_quality",
		"spending", "cash_flow", "recurring_charges", "anomalies", "search_transactions",
		"holdings", "net_worth", "acb", "monthly_summary",
	}, names)
	frames := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.GreaterOrEqual(t, len(frames), 2)
	for _, frame := range frames {
		var message struct {
			JSONRPC string `json:"jsonrpc"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame), &message), frame)
		assert.Equal(t, "2.0", message.JSONRPC, frame)
	}
	assert.Empty(t, stderr.String())
}

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

func Test_newMCPServe_reports_the_build_infos_module_version(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "a release build", info: &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, want: "v9.9.9"},
		{name: "no build info", info: nil, want: mcpTestServerVersion},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			serverStdin, toServer := io.Pipe()
			serverStdout, fromServer := io.Pipe()
			go func() {
				_ = newMCPServe(c.info)(ctx, serverStdin, fromServer, io.Discard, func() {})
				_ = fromServer.Close()
			}()

			client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
			session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
			require.NoError(t, err)
			defer func() { _ = session.Close() }()

			assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: c.want}, session.InitializeResult().ServerInfo)
		})
	}
}

const mcpTerminalHint = "quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop.\n"

func Test_run_mcp_at_a_terminal_prints_the_hint_and_keeps_serving(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(env *cli.Env) {
		env.IsTerminal = func(io.Reader) bool { return true }
	})

	_, listErr := peer.session.ListTools(ctx, nil)
	require.NoError(t, peer.session.Close())
	code := peer.waitForExit(ctx, t)

	require.NoError(t, listErr)
	assert.Equal(t, 0, code)
	assert.Equal(t, mcpTerminalHint, peer.stderr.String())
}

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

// cancelBound is how long a cancelled query has to report its interrupt.
const cancelBound = 10 * time.Second

func Test_run_mcp_cancelled_query_is_interrupted_quietly(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	queries := newQueryRecorder()
	var requests, responses lockedBuffer
	peer := startMCP(ctx, t, func(env *cli.Env) {
		env.Stdin = io.TeeReader(env.Stdin, &requests)
		env.Stdout = io.MultiWriter(env.Stdout, &responses)
		env.ServeMCP = newMCPServe(nil, mcp.WithReport(queries.factory(home)))
	})
	slowCtx, cancelSlow := context.WithCancel(ctx)
	defer cancelSlow()
	slowDone := make(chan error, 1)
	go func() {
		_, err := peer.session.CallTool(slowCtx, &sdk.CallToolParams{Name: "query", Arguments: map[string]any{"sql": slowQuery}})
		slowDone <- err
	}()
	queries.awaitRunning(ctx, t)

	cancelSlow()
	interrupted := queries.nextErrorWithin(ctx, t, cancelBound)
	cancelledID := toolCallID(t, requests.String())
	awaitResponse(ctx, t, &responses, cancelledID)
	control, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "query", Arguments: map[string]any{"sql": "CREATE TABLE notes (body VARCHAR)"}})

	require.NoError(t, err)
	require.ErrorIs(t, interrupted, context.Canceled)
	require.NotErrorIs(t, interrupted, context.DeadlineExceeded)
	assert.True(t, control.IsError)
	assert.Equal(t, "quarry: mcp: query: "+mcpWriteRefusal+"\n", peer.stderr.String())
	require.ErrorIs(t, <-slowDone, context.Canceled)
}

// lockedBuffer is a bytes.Buffer safe to read while the server writes it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// toolCallID is the JSON-RPC id of the one tools/call among the request frames sent so far.
func toolCallID(t *testing.T, requests string) string {
	t.Helper()
	var ids []string
	for frame := range strings.SplitSeq(requests, "\n") {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal([]byte(frame), &message) == nil && message.Method == "tools/call" {
			ids = append(ids, string(message.ID))
		}
	}
	require.Len(t, ids, 1)
	return ids[0]
}

// awaitResponse waits until responses holds a complete frame answering the request with id.
func awaitResponse(ctx context.Context, t *testing.T, responses *lockedBuffer, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		for frame := range strings.SplitSeq(responses.String(), "\n") {
			var message struct {
				ID     json.RawMessage `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if json.Unmarshal([]byte(frame), &message) == nil && string(message.ID) == id && message.Result != nil {
				return true
			}
		}
		return false
	}, time.Until(deadlineOf(ctx)), 10*time.Millisecond)
}

func deadlineOf(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	return deadline
}

// slowQuery runs for far longer than any test waits for it.
const slowQuery = "SELECT sum(a.range * b.range) FROM range(1000000) a, range(1000000) b"

const mcpQueryTimeoutLine = "query stopped after 1 second; aggregate or filter it in SQL, then try again"

func Test_run_mcp_query_stops_at_its_deadline(t *testing.T) {
	const timeout = time.Second
	home := newHome(t)
	syncAccountsFixture(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	queries := newQueryRecorder()
	peer := startMCP(ctx, t, func(env *cli.Env) {
		env.ServeMCP = newMCPServe(nil, mcp.WithTimeout(timeout), mcp.WithReport(queries.factory(home)))
	})

	started := time.Now()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "query", Arguments: map[string]any{"sql": slowQuery}})
	elapsed := time.Since(started)

	require.NoError(t, err)
	assert.LessOrEqual(t, elapsed, timeout+10*time.Second)
	assert.True(t, result.IsError)
	assert.Equal(t, mcpQueryTimeoutLine, textOf(result))
	assert.Equal(t, "quarry: mcp: query: "+mcpQueryTimeoutLine+"\n", peer.stderr.String())
	stopped := queries.nextError(ctx, t)
	require.ErrorIs(t, stopped, context.DeadlineExceeded)
	require.NotErrorIs(t, stopped, context.Canceled)
}

// queryRecorder is a report factory over the real store that signals when a statement starts
// running and records each Query's error.
type queryRecorder struct {
	running chan struct{}
	errs    chan error
}

func newQueryRecorder() *queryRecorder {
	const room = 8
	return &queryRecorder{running: make(chan struct{}, room), errs: make(chan error, room)}
}

// factory builds report servers over the store under home, each reading through r.
func (r *queryRecorder) factory(home string) mcp.ReportFactory {
	return func(context.Context, string) (*report.Server, error) {
		st := duckstore.New(storeDirUnder(home), duckstore.WithOpenReadOnly(r.open))
		recorded := &recordedStore{Store: st, recorder: r}
		return report.NewServer(report.WithStore(recorded), report.WithHome(home)), nil
	}
}

// open opens the store file for real, handing back a connection that signals when its query starts.
func (r *queryRecorder) open(ctx context.Context, path string) (duckstore.ReadDB, error) {
	db, err := duckdb.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, err
	}
	return &signallingDB{ReadDB: db, recorder: r}, nil
}

// signallingDB is a read connection that signals its recorder as each table query starts.
type signallingDB struct {
	duckstore.ReadDB

	recorder *queryRecorder
}

func (d *signallingDB) QueryTable(ctx context.Context, query string, maxRows int) (duckdb.Table, error) {
	d.recorder.running <- struct{}{}
	return d.ReadDB.QueryTable(ctx, query, maxRows)
}

// awaitRunning waits until a statement starts running, failing the test if none does by ctx's deadline.
func (r *queryRecorder) awaitRunning(ctx context.Context, t *testing.T) {
	t.Helper()
	select {
	case <-r.running:
	case <-ctx.Done():
		require.FailNow(t, "no query started")
	}
}

// nextError is the error of the next Query to finish, failing the test if none does by ctx's deadline.
func (r *queryRecorder) nextError(ctx context.Context, t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.errs:
		return err
	case <-ctx.Done():
		require.FailNow(t, "no query finished")
		return nil
	}
}

// nextErrorWithin is nextError, failing the test if no query finishes within bound.
func (r *queryRecorder) nextErrorWithin(ctx context.Context, t *testing.T, bound time.Duration) error {
	t.Helper()
	boundCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	return r.nextError(boundCtx, t)
}

// recordedStore is a report.Store whose Query records its error.
type recordedStore struct {
	report.Store

	recorder *queryRecorder
}

func (s *recordedStore) Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error) {
	result, err := s.Store.Query(ctx, query, maxRows)
	s.recorder.errs <- err
	return result, err
}

func Test_run_mcp_every_tool_refuses_before_the_first_sync(t *testing.T) {
	home := newHome(t)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	wantLine := "no store at " + abbreviated(t, storePathUnder(home), home) + " yet; run quarry sync to build it"
	calls := []struct {
		tool      string
		arguments map[string]any
	}{
		{tool: "query", arguments: map[string]any{"sql": "SELECT 1"}},
		{tool: "describe_schema"},
		{tool: "sync_status"},
		{tool: "data_quality"},
		{tool: "spending"},
		{tool: "cash_flow"},
		{tool: "recurring_charges"},
		{tool: "anomalies"},
		{tool: "search_transactions"},
		{tool: "holdings"},
		{tool: "net_worth"},
		{tool: "acb"},
		{tool: "monthly_summary"},
	}

	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			loggedBefore := peer.stderr.Len()

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Equal(t, wantLine, textOf(result))
			assert.Equal(t, "quarry: mcp: "+c.tool+": "+wantLine+"\n", peer.stderr.String()[loggedBefore:])
		})
	}
}

// directoryStore makes the store path a directory, so the store exists but cannot be opened.
func directoryStore(t *testing.T, home string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(storePathUnder(home), 0o750))
}

// brokenStore syncs a store, then runs each statement against it.
func brokenStore(statements ...string) func(*testing.T, string) {
	return func(t *testing.T, home string) {
		t.Helper()
		syncAccountsFixture(t, home)
		for _, stmt := range statements {
			editStore(t, home, stmt)
		}
	}
}

func Test_run_mcp_logs_only_the_withheld_line_for_a_store_read_fault(t *testing.T) {
	const (
		rebuild    = "; run quarry sync to rebuild it"
		storeToken = "{store}"
	)
	cases := []struct {
		name      string
		tool      string
		arguments map[string]any
		damage    func(*testing.T, string)
		reason    string
	}{
		{
			name: "query, store cannot be opened", tool: "query", arguments: map[string]any{"sql": "SELECT 1"},
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "describe_schema, store cannot be opened", tool: "describe_schema",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "sync_status, store cannot be opened", tool: "sync_status",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "data_quality, store cannot be opened", tool: "data_quality",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "spending, store cannot be opened", tool: "spending",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "cash_flow, store cannot be opened", tool: "cash_flow",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "recurring_charges, store cannot be opened", tool: "recurring_charges",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "anomalies, store cannot be opened", tool: "anomalies",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "search_transactions, store cannot be opened", tool: "search_transactions",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "holdings, store cannot be opened", tool: "holdings",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "holdings, its view dropped", tool: "holdings",
			damage: brokenStore("DROP VIEW v_holdings"), reason: "Table with name v_holdings does not exist!",
		},
		{
			name: "net_worth, store cannot be opened", tool: "net_worth",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "acb, store cannot be opened", tool: "acb",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "monthly_summary, store cannot be opened", tool: "monthly_summary",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "net_worth, its view dropped", tool: "net_worth",
			damage: brokenStore("DROP VIEW v_net_worth"), reason: "Table with name v_net_worth does not exist!",
		},
		{
			name: "describe_schema, a table dropped", tool: "describe_schema",
			damage: brokenStore("DROP TABLE categories CASCADE"), reason: "Table with name categories does not exist!",
		},
		{
			name: "sync_status, import history gone", tool: "sync_status",
			damage: brokenStore("DELETE FROM import_runs"), reason: "the store has no import history",
		},
		{
			name: "monthly_summary, import history gone", tool: "monthly_summary",
			damage: brokenStore("DELETE FROM import_runs"), reason: "the store has no import history",
		},
		{
			name: "data_quality, findings tables dropped", tool: "data_quality",
			damage: brokenStore("DROP TABLE finding_items", "DROP TABLE findings"), reason: "Table with name findings does not exist!",
		},
		{
			name: "spending, its view dropped", tool: "spending",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "cash_flow, its view dropped", tool: "cash_flow",
			damage: brokenStore("DROP VIEW v_cash_flow"), reason: "Table with name v_cash_flow does not exist!",
		},
		{
			name: "recurring_charges, its view dropped", tool: "recurring_charges",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "anomalies, its view dropped", tool: "anomalies",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "search_transactions, a table dropped", tool: "search_transactions",
			damage: brokenStore("DROP TABLE categories CASCADE"), reason: "Table with name categories does not exist!",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			c.damage(t, home)
			at := abbreviated(t, storePathUnder(home), home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, "cannot read the store at "+at+": "+strings.ReplaceAll(c.reason, storeToken, at)+rebuild, textOf(result))
			assert.Equal(t, "quarry: mcp: "+c.tool+": cannot read the store at "+at+"; details went to the client only\n", peer.stderr.String())
		})
	}
}

const mcpInstructions = `quarry serves the user's Quicken Classic for Mac data from a local,
read-only store. Call sync_status first and tell the user how old the
snapshot is (snapshot.taken_at). For spending, income, recurring charges
and unusually large charges call spending, cash_flow, recurring_charges
and anomalies: they apply quarry's rules for transfers, refunds and
currencies. To find particular transactions by payee, memo, amount or
date call search_transactions. For other questions call describe_schema
before writing SQL for query: its conventions say which views already
leave out transfers and how amounts, signs and currencies work. Every
number you report must come from a tool result; never estimate. quarry
cannot change data: fixes are made in Quicken, then the user runs quarry
sync.`

const mcpQueryDescription = `Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income totals call spending or
cash_flow instead, and to find transactions by payee, memo or amount call
search_transactions; in SQL use v_spending and v_cash_flow, which already
leave out transfers between the user's own accounts. Returns at most
` + "`limit`" + ` rows (default 500, the most allowed); aggregate in SQL rather
than paging through rows. The store cannot be changed, and other files,
databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.`

const mcpSyncStatusDescription = `Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.`

const mcpDescribeSchemaDescription = `Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.`

const mcpDataQualityDescription = `List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories, shares added with no cost), and investment accounts
not yet listed as registered or non-registered in quarry's config file,
which acb needs. Each finding has an id, the suggested fix, and the
transactions, accounts, payees or categories it is about. quarry never
fixes them: the user fixes them in Quicken and runs quarry sync, or adds
the account to quarry's config file, and they drop off. To ignore a
finding the user adds its id to findings.ignore in quarry's config file.`

const mcpSpendingDescription = `Total the user's spending for a period, grouped by category, payee, tag
or month, with a total per currency. quarry's spending rules apply:
transfers between the user's own accounts, Quicken's system categories,
transactions marked "exclude from reports" and accounts Quicken leaves out
of reports are not counted, and refunds are netted, so a category can come
out negative. Each split is converted at the Bank of Canada rate for its
date. Use this rather than query for spending totals. Returns at most 500
rows; totals always count every row.`

const mcpCashFlowDescription = `Report income, spending, net and savings rate for each month or year of a
period, with totals per currency. The rules are spending's: transfers
between the user's own accounts are neither income nor spending, and spent
equals spending's total for the same period, accounts and currency.
Savings rate is net divided by income, null when income is zero or less.
A period that since or until cuts short is marked partial.`

const mcpRecurringDescription = `List charges that repeat every week, month, quarter or year at a steady
amount (subscriptions, memberships, insurance), found in all of the
user's history and listed when they were running during the period. Each
series has its cadence, latest amount, cost per year while active, price
changes and accounts. A series is found in its account's own currency, so
an exchange-rate change is never a price change. Charges dated after today
never count. Bills whose amount changes most times, such as utilities, are
not listed; use spending with by payee for those. Returns at most 500
series; totals count every series.`

const mcpAnomaliesDescription = `List charges in the period that are unusually large: more than 2 times the
median of the payee's earlier charges (when it has at least 3), else more
than 5 times the median of the category's earlier charges (at least 10).
Charges under 100.00 in their account's own currency are never listed.
Each charge is compared with all earlier history, whatever the period.
Possible duplicates are not listed here; data_quality lists them. Charges
dated after today are never listed. Returns at most 500 charges, newest
first.`

const mcpSearchDescription = `Find the user's transactions by text, date, account, category or amount,
newest first. text matches payee names, transaction memos and split
memos, ignoring letter case; % and _ are plain characters. Every
transaction is searched, including transfers between the user's own
accounts (flagged transfer) and transactions Quicken's reports leave out
(flagged excluded); spending and cash_flow do not count those, so call
them for totals rather than adding up these rows. Amounts are in each
account's own currency and are never converted. Returns at most limit
transactions (default 500); matched counts every match.`

const mcpHoldingsDescription = "Securities held on one day with share count, latest price and its date, and value; cash in investment accounts is not included."

const mcpHoldingsInputSchema = `{
	"type": "object",
	"properties": {
		"as_of": {"type": "string", "description": "Day to value holdings on: YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day. Defaults to today."},
		"accounts": {"type": "array", "items": {"type": "string"}, "description": "List only these accounts, each given by id or by name in any letter case. ` +
	`Omit it for every account."},
		"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
	`Defaults to reporting.currency in quarry's config file, else CAD."}
	},
	"additionalProperties": false
}`

const mcpNetWorthDescription = "Net worth on one day (as_of, default today) or at each month end from since to until, by account type and currency; " +
	"brokerage and retirement accounts count their cash plus their holdings' value."

const mcpNetWorthInputSchema = `{
	"type": "object",
	"properties": {
		"as_of": {"type": "string", "description": "Day to value net worth on: YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day. Defaults to today. Cannot be combined with since or until."},
		"since": {"type": "string", "description": "List net worth at each month end on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to January 1 of this year when until is given."},
		"until": {"type": "string", "description": "List net worth at each month end on or before this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to today; a later date means today."},
		"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
	`Defaults to reporting.currency in quarry's config file, else CAD."}
	},
	"additionalProperties": false
}`

const mcpACBDescription = "Adjusted cost base and realized capital gains per tax year, in CAD, the way the CRA defines them: average cost per security " +
	"pooled across non-registered accounts; possible superficial losses marked, not adjusted. A worksheet, not a filing."

const mcpACBInputSchema = `{
	"type": "object",
	"properties": {
		"year": {"type": "integer", "minimum": 1, "maximum": 9999, "description": "Tax year to report, such as 2024, up to this year: years lists only that year (even with no sale), ` +
	`and securities only those with a sale, or a return of capital above ACB, in it; each security's events stay its full history. Omit it for every year."},
		"security": {"type": "array", "items": {"type": "string"}, "description": "Report only these securities, each given by id, ticker or name in any letter case; ` +
	`years and securities count only them. A security held only in registered accounts has no ACB and is left out, with a warning. Omit it for every security."}
	},
	"additionalProperties": false
}`

const mcpMonthlySummaryDescription = "Summarize one month (default last month): data freshness, findings counts, unusually large charges, " +
	"recurring charges new in the month, and net worth at the month end beside the month before, with the change. " +
	"The same document quarry summary --json prints. Read-only; never syncs."

const mcpMonthlySummaryInputSchema = `{
	"type": "object",
	"properties": {
		"month": {"type": "string", "description": "Month to summarize: YYYY-MM, a month that has ended. Defaults to last month."},
		"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
	`Defaults to reporting.currency in quarry's config file, else CAD."}
	},
	"additionalProperties": false
}`

const (
	mcpRecurringInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "List series still running on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "List series that started on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "List only series with a charge in one of these accounts, each given by id or by name in any letter case. ` +
		`Omit it for every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."}
		},
		"additionalProperties": false
	}`
	mcpAnomaliesInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "List charges dated on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "List charges dated on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "List only charges in these accounts, each given by id or by name in any letter case; ` +
		`the payee's charges in other accounts still count as history."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."}
		},
		"additionalProperties": false
	}`
)

const (
	mcpCashFlowInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today; future-dated transactions count only when until is later than today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Count only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to count every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."},
			"by": {"type": "string", "enum": ["month", "year"], "default": "month", "description": "One row per month (the default) or per year."}
		},
		"additionalProperties": false
	}`
	mcpSpendingInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today; future-dated transactions count only when until is later than today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Count only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to count every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."},
			"by": {"type": "string", "enum": ["category", "payee", "tag", "month"], "default": "category", "description": "Group by category (the default), payee, tag or month. ` +
		`A split with several tags counts under each tag."}
		},
		"additionalProperties": false
	}`
	mcpQueryInputSchema = `{
		"type": "object",
		"properties": {
			"sql":   {"type": "string", "minLength": 1, "description": "One read-only SQL statement in DuckDB's dialect over quarry's tables and views; describe_schema lists them."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "default": 500, "description": "Most rows to return, 1 to 500. Defaults to 500."}
		},
		"required": ["sql"],
		"additionalProperties": false
	}`
	mcpNoInputSchema          = `{"type": "object", "additionalProperties": false}`
	mcpDataQualityInputSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["open", "ignored", "fixed", "all"], "default": "open",
				"description": "Which findings to list: open (the default), ignored (the user listed the id in findings.ignore), fixed (no longer found since a later sync; ` +
		`never unclassified-account or shares-without-cost, which just leave the list), or all."},
			"type":   {"type": "string", "enum": [
				"duplicate", "one-sided-transfer", "unlinked-transfer", "uncategorized",
				"mixed-categories", "payee-variants", "similar-categories", "unused-category", "unclassified-account",
				"shares-without-cost"], "description": "List only findings of this type. Omit it to list every type."},
			"limit":  {"type": "integer", "minimum": 1, "maximum": 500, "default": 50,
				"description": "Most findings to return, 1 to 500. Defaults to 50. counts always covers every finding, and each finding lists at most 25 items."}
		},
		"additionalProperties": false
	}`
	mcpSearchInputSchema = `{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "Words to find in payee names, transaction memos and split memos, in any letter case; every character is literal. ` +
		`Omit it to search by the other parameters alone."},
			"since": {"type": "string", "description": "Earliest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Omit it to search from the first transaction."},
			"until": {"type": "string", "description": "Latest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Omit it to search every later date, future-dated transactions included."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Search only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to search every account."},
			"category": {"type": "string", "description": "List only transactions with a split in this category or one under it, given by its full path (such as Food:Groceries) in any letter case."},
			"min": {"type": "string", "description": "Smallest amount to list, as a string such as \"25\" or \"19.99\", compared without its sign in the account's own currency."},
			"max": {"type": "string", "description": "Largest amount to list, as a string such as \"100\" or \"250.50\", compared without its sign in the account's own currency. ` +
		`Give min and max the same value to find one amount."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "default": 500,
				"description": "Most transactions to return, newest first, 1 to 500. Defaults to 500. matched always counts every match."}
		},
		"additionalProperties": false
	}`
	mcpObjectOutputSchema = `{"type": "object"}`
)

func Test_run_mcp_describes_every_tool(t *testing.T) {
	t.Run("tools/list and instructions", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
		defer cancel()
		peer := startMCP(ctx, t, func(*cli.Env) {})
		session := peer.session

		listed, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, session.Close())
		peer.waitForExit(ctx, t)

		assert.Equal(t, mcpInstructions, session.InitializeResult().Instructions)
		wantTools := map[string]struct{ description, inputSchema string }{
			"query":               {mcpQueryDescription, mcpQueryInputSchema},
			"describe_schema":     {mcpDescribeSchemaDescription, mcpNoInputSchema},
			"sync_status":         {mcpSyncStatusDescription, mcpNoInputSchema},
			"data_quality":        {mcpDataQualityDescription, mcpDataQualityInputSchema},
			"spending":            {mcpSpendingDescription, mcpSpendingInputSchema},
			"cash_flow":           {mcpCashFlowDescription, mcpCashFlowInputSchema},
			"recurring_charges":   {mcpRecurringDescription, mcpRecurringInputSchema},
			"anomalies":           {mcpAnomaliesDescription, mcpAnomaliesInputSchema},
			"search_transactions": {mcpSearchDescription, mcpSearchInputSchema},
			"holdings":            {mcpHoldingsDescription, mcpHoldingsInputSchema},
			"net_worth":           {mcpNetWorthDescription, mcpNetWorthInputSchema},
			"acb":                 {mcpACBDescription, mcpACBInputSchema},
			"monthly_summary":     {mcpMonthlySummaryDescription, mcpMonthlySummaryInputSchema},
		}
		require.Len(t, listed.Tools, len(wantTools))
		for _, tool := range listed.Tools {
			want, known := wantTools[tool.Name]
			require.True(t, known, "unexpected tool %q", tool.Name)
			assert.Equal(t, want.description, tool.Description, tool.Name)
			assertJSONEqualAny(t, want.inputSchema, tool.InputSchema, tool.Name+" input schema")
			assertJSONEqualAny(t, mcpObjectOutputSchema, tool.OutputSchema, tool.Name+" output schema")
		}
	})

	t.Run("mcp --help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		code := runWith(t.Context(), []string{"mcp", "--help"}, testEnv(&stdout, &stderr))

		assert.Equal(t, 0, code)
		assert.Contains(t, stdout.String(), "SQL runs read-only, and every list a tool returns\nstops at 500 entries.")
		assert.Contains(t, stdout.String(), "Tools: describe_schema, query, sync_status, data_quality, spending,\n"+
			"cash_flow, recurring_charges, anomalies, search_transactions, holdings,\nnet_worth, acb, monthly_summary.")
		assert.Empty(t, stderr.String())
	})
}

// assertJSONEqualAny compares want to got, a decoded JSON value, as JSON.
func assertJSONEqualAny(t *testing.T, want string, got any, msg string) {
	t.Helper()
	encoded, err := json.Marshal(got)
	require.NoError(t, err, msg)
	assert.JSONEq(t, want, string(encoded), msg)
}

// toolClock is the instant both the CLI and the MCP server take as "now".
var toolClock = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// warningsMember is the key every document ends with, so the bytes before it are the document minus its warnings.
const warningsMember = `,"warnings":`

// toolDocumentRun is one CLI --json command and the tool call that must return its document.
type toolDocumentRun struct {
	store     func(t *testing.T, home string)
	config    string
	cliArgs   []string
	tool      string
	arguments map[string]any
	now       time.Time // both surfaces' clock; zero is toolClock
}

// toolDocuments are both surfaces' answers to one run: the compact document minus its warnings, and the warnings.
type toolDocuments struct {
	cliBody, toolBody         string
	cliWarnings, toolWarnings []string
}

// runBothSurfaces runs c's CLI command with --json and c's tool call over one store, the clock fixed at c.now.
func runBothSurfaces(t *testing.T, c toolDocumentRun) toolDocuments {
	t.Helper()
	home := newHome(t)
	c.store(t, home)
	if c.config != "" {
		writeConfig(t, home, c.config)
	}
	now := cmp.Or(c.now, toolClock)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runWith(ctx, slices.Concat(c.cliArgs, []string{"--json"}), spendEnvAt(&stdout, &stderr, now)), stderr.String())
	peer := startMCPAt(ctx, t, now)
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)
	require.False(t, result.IsError, textOf(result))
	require.Empty(t, peer.stderr.String())

	cliBody, cliWarnings := splitWarnings(t, compactJSON(t, stdout.String()))
	toolBody, toolWarnings := splitWarnings(t, textOf(result))
	return toolDocuments{cliBody: cliBody, toolBody: toolBody, cliWarnings: cliWarnings, toolWarnings: toolWarnings}
}

// startClockedMCP connects a client to quarry mcp over the HOME the test set, its clock fixed at toolClock.
func startClockedMCP(ctx context.Context, t *testing.T) *mcpPeer {
	t.Helper()
	return startMCPAt(ctx, t, toolClock)
}

// startMCPAt is startClockedMCP with the clock fixed at now.
func startMCPAt(ctx context.Context, t *testing.T, now time.Time) *mcpPeer {
	t.Helper()
	return startMCP(ctx, t, func(env *cli.Env) {
		env.ServeMCP = newMCPServe(nil, mcp.WithClock(func() time.Time { return now }))
	})
}

// splitWarnings cuts a compact document into its bytes before the warnings member, closed again, and the warnings.
func splitWarnings(t *testing.T, compact string) (string, []string) {
	t.Helper()
	at := strings.LastIndex(compact, warningsMember)
	require.Positive(t, at, compact)
	var warnings []string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(compact[at+len(warningsMember):], "}")), &warnings))
	return compact[:at] + "}", warnings
}

// inToolWords is the CLI warnings with cliWord replaced by tool where the lines say "so <cliWord> leaves it out".
func inToolWords(cliWarnings []string, cliWord, tool string) []string {
	mapped := make([]string, len(cliWarnings))
	for i, line := range cliWarnings {
		mapped[i] = strings.ReplaceAll(line, ", so "+cliWord+" leaves it out", ", so "+tool+" leaves it out")
	}
	return mapped
}

// refuseAccountKeepingItsNameOffStderr calls tool with one unreadable accounts entry per case and asserts the
// client text names the argument while stderr carries only the class line behind logPrefix.
func refuseAccountKeepingItsNameOffStderr(t *testing.T, tool, logPrefix string) {
	t.Helper()
	cases := []struct {
		name       string
		arg        string
		want       string
		wantStderr string
		absent     []string
	}{
		{
			name: "no account has the name", arg: "Nope",
			want:       `no account named "Nope"; call describe_schema to list the accounts`,
			wantStderr: logPrefix + unknownAccountLog + "\n",
			absent:     []string{"Nope"},
		},
		{
			name: "the name is empty", arg: "",
			want:       `no account named ""; call describe_schema to list the accounts`,
			wantStderr: logPrefix + unknownAccountLog + "\n",
			absent:     []string{`""`},
		},
		{
			name: "two accounts share the name", arg: "Visa",
			want:       `2 accounts are named "Visa"; pass one of their ids instead: acct-812, acct-977`,
			wantStderr: logPrefix + ambiguousAccountLog + "\n",
			absent:     []string{"Visa", "acct-812", "acct-977"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
				Name: tool, Arguments: map[string]any{"accounts": []string{c.arg}},
			})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, c.wantStderr, peer.stderr.String())
			for _, text := range c.absent {
				assert.NotContains(t, peer.stderr.String(), text)
			}
		})
	}
}
