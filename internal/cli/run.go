package cli

import (
	"context"
	"errors"
	"io"

	"github.com/koblas/quarry/internal/snapshot"
)

// Execute parses args against quarry's command tree and runs the matched
// command against srv, writing to stdout/stderr and resolving `~` paths
// against home. It returns a UsageError unchanged, an srv.Sync error
// unwrapped, or wraps any other error as a UsageError with a run-sync-help
// hint appended.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, srv *snapshot.Server, home string) error {
	var jsonOut bool
	root := newRootCommand(srv, home, &jsonOut)
	// args must be non-nil: cobra falls back to the process's own os.Args for a nil slice.
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return nil
	}

	var ue UsageError
	if errors.As(err, &ue) {
		return ue
	}
	var re *runtimeError
	if errors.As(err, &re) {
		return re.err
	}
	return UsageError{msg: err.Error() + "; Run 'quarry sync --help' for usage."}
}
