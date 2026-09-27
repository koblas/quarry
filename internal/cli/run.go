package cli

import (
	"context"
	"errors"
	"io"

	"github.com/koblas/quarry/internal/snapshot"
)

// Execute parses args against quarry's command tree and runs the matched
// command against srv, writing to stdout/stderr and resolving `~` paths
// against home. A UsageError, whether from argument validation or already
// wrapped below, is returned unchanged; an error from srv.Sync passes
// through unwrapped (exit-1 default in cmd/quarry); any other cobra-native
// parse or dispatch error is wrapped into a UsageError with a
// "Run 'quarry sync --help'" hint appended.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, srv *snapshot.Server, home string) error {
	if args == nil {
		args = []string{}
	}

	var jsonOut bool
	root := newRootCommand(srv, home, &jsonOut)
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
