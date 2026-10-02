// White-box: handler's cancel arm needs a run failing under a cancelled context, which a client cannot arrange
// because the server suppresses the answer to a call the client cancelled.
package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDriverInterrupt = errors.New("interrupt error: interrupted")

type stoppedCase struct {
	name    string
	stopped stoppedFunc
	want    string
}

func stoppedCases() []stoppedCase {
	return []stoppedCase{
		{
			name: "query", stopped: queryStoppedLine,
			want: "query stopped after 2 seconds; aggregate or filter it in SQL, then try again",
		},
		{name: "sync_status", stopped: stoppedLine("sync_status"), want: "sync_status stopped after 2 seconds; try again"},
	}
}

// interruptedRun is a tool run that fails as the store does once its context has ended.
func interruptedRun(ctx context.Context, _ struct{}) (any, error) {
	return nil, store.InterruptedBy(ctx, errDriverInterrupt)
}

func Test_handler_passes_a_cancel_on_as_its_own_error_not_the_timeout_line(t *testing.T) {
	for _, c := range stoppedCases() {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			call := handler(2*time.Second, c.stopped, interruptedRun)

			_, _, err := call(ctx, nil, struct{}{})

			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			assert.NotContains(t, err.Error(), "stopped after")
		})
	}
}

func Test_handler_answers_a_deadline_with_the_tools_timeout_line(t *testing.T) {
	for _, c := range stoppedCases() {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
			defer cancel()
			call := handler(2*time.Second, c.stopped, interruptedRun)

			_, _, err := call(ctx, nil, struct{}{})

			assert.EqualError(t, err, c.want)
		})
	}
}
