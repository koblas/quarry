package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowQuery runs for far longer than any test waits for it.
const slowQuery = "SELECT sum(a.range * b.range) FROM range(1000000) a, range(1000000) b"

const mcpQueryTimeoutLine = "query stopped after 1 second; aggregate or filter it in SQL, then try again"

func Test_run_mcp_query_stops_at_its_deadline(t *testing.T) {
	const timeout = time.Second
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	queries := newQueryRecorder()
	peer := startMCP(ctx, t, func(env *cli.Env) {
		env.ServeMCP = newMCPServe(nil, mcp.WithTimeout(timeout), mcp.WithReport(queries.factory(home)))
	})
	callCtx, cancelCall := context.WithTimeout(ctx, timeout+time.Second)
	defer cancelCall()

	started := time.Now()
	result, err := peer.session.CallTool(callCtx, &sdk.CallToolParams{Name: "query", Arguments: map[string]any{"sql": slowQuery}})
	elapsed := time.Since(started)

	require.NoError(t, err)
	assert.LessOrEqual(t, elapsed, timeout+time.Second)
	assert.True(t, result.IsError)
	assert.Equal(t, mcpQueryTimeoutLine, textOf(result))
	assert.Equal(t, "quarry: mcp: query: "+mcpQueryTimeoutLine+"\n", peer.stderr.String())
	stopped := queries.nextError(ctx, t)
	require.ErrorIs(t, stopped, context.DeadlineExceeded)
	require.NotErrorIs(t, stopped, context.Canceled)
}

// queryRecorder is a report factory over the real store that signals each Query's start and records its error.
type queryRecorder struct {
	started chan struct{}
	errs    chan error
}

func newQueryRecorder() *queryRecorder {
	const room = 8
	return &queryRecorder{started: make(chan struct{}, room), errs: make(chan error, room)}
}

// factory builds report servers over the store under home, each reading through r.
func (r *queryRecorder) factory(home string) mcp.ReportFactory {
	return func(context.Context, string) (*report.Server, error) {
		recorded := &recordedStore{Store: duckstore.New(storeDirUnder(home)), recorder: r}
		return report.NewServer(report.WithStore(recorded), report.WithHome(home)), nil
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

// recordedStore is a report.Store whose Query reports to its recorder.
type recordedStore struct {
	report.Store

	recorder *queryRecorder
}

func (s *recordedStore) Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error) {
	s.recorder.started <- struct{}{}
	result, err := s.Store.Query(ctx, query, maxRows)
	s.recorder.errs <- err
	return result, err
}
