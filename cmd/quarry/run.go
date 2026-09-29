package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/importer"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
)

// These guards document that the wired types satisfy each consumer's port with no adapter.
var (
	_ snapshot.Importer   = (*importer.Server)(nil)
	_ importer.Store      = (*duckstore.Store)(nil)
	_ snapshot.StoreProbe = (*duckstore.Store)(nil)
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

// newServerFactory returns cli.Execute's ServerFactory: it resolves the
// home directory and the embedded reference schema, then builds the Server
// against them, passing storeOpts to the store it builds.
func newServerFactory(storeOpts ...duckstore.Option) cli.ServerFactory {
	return func(ctx context.Context) (*snapshot.Server, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, errHomeDirectory
		}

		ref, err := v9.Reference(ctx)
		if err != nil {
			return nil, snapshot.FailureOutcome(ctx, err)
		}

		storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
		info, _ := debug.ReadBuildInfo()
		st := duckstore.New(storeDir, append([]duckstore.Option{duckstore.WithQuarryVersion(buildVersion(info))}, storeOpts...)...)
		srv := snapshot.NewServer(
			snapshot.WithSnapshotDir(filepath.Join(storeDir, "snapshots")),
			snapshot.WithReference(v9.ReferenceLabel, ref),
			snapshot.WithHome(home),
			snapshot.WithImporter(importer.NewServer(importer.WithStore(st))),
			snapshot.WithStoreProbe(st),
		)
		return srv, nil
	}
}

// buildVersion is the main module's version from info, "" when info is nil
// or carries none.
func buildVersion(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	return info.Main.Version
}

// errHomeDirectory is sync's fixed-literal refusal when the home directory
// cannot be resolved; os.UserHomeDir's own text varies by platform.
var errHomeDirectory = errors.New("cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again")

// run is the process entrypoint's testable body: it delegates to
// cli.Execute, returning the process exit code (0 success, 1 failure, 2 usage).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, stdout, stderr, newServerFactory())
}

// runWith is run against an explicit ServerFactory.
func runWith(ctx context.Context, args []string, stdout, stderr io.Writer, newServer cli.ServerFactory) int {
	if err := cli.Execute(ctx, args, stdout, stderr, newServer); err != nil {
		_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)

		if _, ok := errors.AsType[cli.UsageError](err); ok {
			return 2
		}
		return 1
	}
	return 0
}
