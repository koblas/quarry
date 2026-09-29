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
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
)

// These guards document that the wired types satisfy each consumer's port with no adapter.
var (
	_ snapshot.Importer   = (*importer.Server)(nil)
	_ importer.Store      = (*duckstore.Store)(nil)
	_ snapshot.StoreProbe = (*duckstore.Store)(nil)
	_ report.Store        = (*duckstore.Store)(nil)
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
		home, err := resolveHome("sync")
		if err != nil {
			return nil, err
		}

		ref, err := v9.Reference(ctx)
		if err != nil {
			return nil, snapshot.FailureOutcome(ctx, err)
		}

		storeDir := storeDirUnder(home)
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

// newReportFactory returns cli.Execute's ReportFactory: it resolves the home
// directory and builds a report.Server over the store there, passing
// storeOpts to it. It never creates the store or its directory.
func newReportFactory(storeOpts ...duckstore.Option) cli.ReportFactory {
	return func(_ context.Context, command string) (*report.Server, error) {
		home, err := resolveHome(command)
		if err != nil {
			return nil, err
		}

		st := duckstore.New(storeDirUnder(home), storeOpts...)
		return report.NewServer(report.WithStore(st), report.WithHome(home)), nil
	}
}

// storeDirUnder is the directory holding quarry's store and snapshots.
func storeDirUnder(home string) string {
	return filepath.Join(home, "Library", "Application Support", "quarry")
}

// resolveHome returns the user's home directory, or the refusal telling
// them to set HOME and run command again; os.UserHomeDir's own text varies
// by platform.
func resolveHome(command string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your home directory ($HOME is not set); set HOME, then run quarry %s again", command) //nolint:err113 // fixed user copy; the platform error names no fix
	}
	return home, nil
}

// defaultEnv is the process's wiring: the real stdin, the given output
// streams, and the factories over the default store.
func defaultEnv(stdout, stderr io.Writer) cli.Env {
	return cli.Env{
		Stdin:     os.Stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		NewServer: newServerFactory(),
		NewReport: newReportFactory(),
	}
}

// run is the process entrypoint's testable body: it delegates to
// cli.Execute, returning the process exit code (0 success, 1 failure, 2 usage).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, defaultEnv(stdout, stderr))
}

// runWith is run against an explicit Env.
func runWith(ctx context.Context, args []string, env cli.Env) int {
	if err := cli.Execute(ctx, args, env); err != nil {
		_, _ = fmt.Fprintf(env.Stderr, "quarry: %s\n", err)

		if _, ok := errors.AsType[cli.UsageError](err); ok {
			return 2
		}
		return 1
	}
	return 0
}
