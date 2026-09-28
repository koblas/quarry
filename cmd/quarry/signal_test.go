// signalContext is unexported, so its tests live in package main rather
// than importing main from outside.
package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

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
