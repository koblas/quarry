package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cancelBound is how long a cancelled query has to report its interrupt.
const cancelBound = 3 * time.Second

func Test_run_mcp_cancelled_query_is_interrupted_quietly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	<-queries.running

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
