package main

import (
	"context"
	"os"
)

func main() {
	// unreachable: main is the process entrypoint; signalContext and run are exercised directly by their own tests.
	ctx, stop := signalContext(context.Background())
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
