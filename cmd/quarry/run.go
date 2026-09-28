package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
)

// signalContext wraps parent with SIGINT/SIGTERM handling: ctx.Done() closes
// on either signal, and a goroutine calls stop once it does, so a second
// signal falls through to the OS default instead of staying diverted.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx, stop
}

// newServer is cli.Execute's ServerFactory: it resolves the home directory
// and the embedded reference schema, then builds the Server against them.
func newServer(ctx context.Context) (*snapshot.Server, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", homeDirectoryRefusal(err)
	}

	ref, err := v9.Reference(ctx)
	if err != nil {
		return nil, "", snapshot.FailureOutcome(ctx, err)
	}

	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
	)
	return srv, home, nil
}

// homeDirectoryRefusal reports that the home directory could not be
// resolved: every path sync touches derives from it, so this refusal only
// ever fires from inside sync's RunE.
func homeDirectoryRefusal(err error) error {
	return fmt.Errorf("cannot find your home directory (%s); set HOME, then run quarry sync again", err)
}

// run is the process entrypoint's testable body: it delegates to
// cli.Execute, returning the process exit code (0 success, 1 failure, 2 usage).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := cli.Execute(ctx, args, stdout, stderr, newServer); err != nil {
		_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)

		var ue cli.UsageError
		if errors.As(err, &ue) {
			return 2
		}
		return 1
	}
	return 0
}
