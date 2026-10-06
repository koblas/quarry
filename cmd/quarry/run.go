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
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/lockfile"
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
	_ snapshot.Locker     = (*lockfile.Locker)(nil)
	_ report.Store        = (*duckstore.Store)(nil)

	_ duckstore.RatesSource = (*fx.Server)(nil)
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
// against them, passing storeOpts to the store it builds and appending the
// Server options the caller passes.
func newServerFactory(storeOpts ...duckstore.Option) cli.ServerFactory {
	return func(ctx context.Context, opts ...snapshot.Option) (*snapshot.Server, error) {
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
		st := duckstore.New(storeDir, append([]duckstore.Option{
			duckstore.WithQuarryVersion(buildVersion(info)), duckstore.WithRates(fx.NewServer()),
		}, storeOpts...)...)
		srv := snapshot.NewServer(append([]snapshot.Option{
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithReference(v9.ReferenceLabel, ref),
			snapshot.WithHome(home),
			snapshot.WithImporter(importer.NewServer(importer.WithStore(st))),
			snapshot.WithStoreProbe(st),
			snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModeSync)),
		}, opts...)...)
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

// newSnapshotsFactory returns cli.Execute's SnapshotsFactory: a Server over the snapshots
// folder and the store's probe, with no reference schema and no importer.
func newSnapshotsFactory(storeOpts ...duckstore.Option) cli.SnapshotsFactory {
	return func(_ context.Context, command string) (*snapshot.Server, error) {
		home, err := resolveHome(command)
		if err != nil {
			return nil, err
		}

		storeDir := storeDirUnder(home)
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithStoreProbe(duckstore.New(storeDir, storeOpts...)),
		), nil
	}
}

// newConfigLoader returns cli.Execute's ConfigLoader: it resolves the home
// directory and loads the config file in quarry's store directory.
func newConfigLoader() cli.ConfigLoader {
	return func(command string) (config.Config, error) {
		home, err := resolveHome(command)
		if err != nil {
			return config.Config{}, err
		}

		return config.Load(home, filepath.Join(storeDirUnder(home), "config.toml"))
	}
}

// newMCPServe returns cli.Execute's MCPServeFunc: an MCP server reporting the module version,
// reading the store through newReportFactory, then opts. It refuses without calling ready when $HOME is unset.
func newMCPServe(info *debug.BuildInfo, opts ...mcp.Option) cli.MCPServeFunc {
	return func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, ready func()) error {
		if _, err := resolveHome("mcp"); err != nil {
			return err
		}
		srv := mcp.NewServer(append([]mcp.Option{
			mcp.WithVersion(buildVersion(info)),
			mcp.WithReport(mcp.ReportFactory(newReportFactory())),
			mcp.WithConfig(mcp.ConfigLoader(newConfigLoader())),
		}, opts...)...)
		// A broken stdout must surface as EPIPE from the write, not kill the process.
		signal.Ignore(syscall.SIGPIPE)
		ready()
		return srv.Serve(ctx, stdin, stdout, stderr)
	}
}

// storeDirUnder is the directory holding quarry's store and snapshots.
func storeDirUnder(home string) string {
	return filepath.Join(home, "Library", "Application Support", "quarry")
}

// lockPathUnder is the writer lock's file in the store directory storeDir.
func lockPathUnder(storeDir string) string {
	return filepath.Join(storeDir, "quarry.lock")
}

// snapshotsDirUnder is the directory holding the snapshots quarry has taken.
func snapshotsDirUnder(home string) string {
	return filepath.Join(storeDirUnder(home), "snapshots")
}

// errNoHome is the home-directory refusal's lead; resolveHome appends what to do.
var errNoHome = errors.New("cannot find your home directory ($HOME is not set)")

// resolveHome returns the user's home directory, or the refusal telling
// them to set HOME and run command again; os.UserHomeDir's own text varies
// by platform.
func resolveHome(command string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w; set HOME, then run quarry %s again", errNoHome, command)
	}
	return home, nil
}

// defaultEnv is the process's wiring: the real stdin, the given output
// streams, and the factories over the default store.
func defaultEnv(stdout, stderr io.Writer) cli.Env {
	info, _ := debug.ReadBuildInfo()
	return cli.Env{
		Stdin:        os.Stdin,
		Stdout:       stdout,
		Stderr:       stderr,
		NewServer:    newServerFactory(),
		NewReport:    newReportFactory(),
		NewSnapshots: newSnapshotsFactory(),
		LoadConfig:   newConfigLoader(),
		ServeMCP:     newMCPServe(info),
		IsTerminal:   isTerminal,
		Now:          time.Now,
	}
}

// runProcess is the process entrypoint's testable body: it delegates to
// cli.Execute over the real wiring, returning the process exit code (0 success, 1 failure, 2 usage).
func runProcess(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, defaultEnv(stdout, stderr))
}

// runWith is runProcess against an explicit Env.
func runWith(ctx context.Context, args []string, env cli.Env) int {
	return exitCode(cli.Execute(ctx, args, env), env.Stderr)
}

// exitCode is the process exit code for err (0 for nil, 2 for a usage error, else 1),
// printing err to stderr unless the command already reported it.
func exitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	if _, ok := errors.AsType[cli.ReportedError](err); ok {
		return 1
	}
	_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)

	if _, ok := errors.AsType[cli.UsageError](err); ok {
		return 2
	}
	return 1
}
