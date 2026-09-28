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
	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
)

// var _ documents that importer.Server satisfies snapshot.Importer with no adapter.
var _ snapshot.Importer = (*importer.Server)(nil)

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

// newServerFactory returns cli.Execute's ServerFactory: it resolves the
// home directory and the embedded reference schema, then builds the Server
// against them, passing storeOpts to the store it builds.
func newServerFactory(storeOpts ...duckstore.Option) cli.ServerFactory {
	return func(ctx context.Context) (*snapshot.Server, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, homeDirectoryRefusal()
		}

		ref, err := v9.Reference(ctx)
		if err != nil {
			return nil, snapshot.FailureOutcome(ctx, err)
		}

		storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
		srv := snapshot.NewServer(
			snapshot.WithSnapshotDir(filepath.Join(storeDir, "snapshots")),
			snapshot.WithReference(v9.ReferenceLabel, ref),
			snapshot.WithHome(home),
			snapshot.WithImporter(importer.NewServer(importer.WithStore(duckstore.New(storeDir, storeOpts...)))),
			snapshot.WithStorePath(filepath.Join(storeDir, duckstore.FileName)),
		)
		return srv, nil
	}
}

// homeDirectoryRefusal is sync's fixed-literal refusal when the home
// directory cannot be resolved; os.UserHomeDir's own text varies by platform.
func homeDirectoryRefusal() error {
	return errors.New("cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again")
}

// run is the process entrypoint's testable body: it delegates to
// cli.Execute, returning the process exit code (0 success, 1 failure, 2 usage).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, stdout, stderr, newServerFactory())
}

// runWith is run against an explicit ServerFactory.
func runWith(ctx context.Context, args []string, stdout, stderr io.Writer, newServer cli.ServerFactory) int {
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
