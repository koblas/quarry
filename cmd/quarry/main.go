package main

import (
	"context"
	"os"
)

func main() {
	// unreachable: main is the process entrypoint; signalContext and runProcess are exercised directly by their own tests.
	ctx, stop := signalContext(context.Background())
	code := runProcess(ctx, os.Args[1:], os.Stdout, os.Stderr) // unreachable: main is the process entrypoint; runProcess is exercised directly by its own test.
	stop()
	os.Exit(code)
}
