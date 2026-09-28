package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// unreachable: main is the process entrypoint; run is exercised directly by its own tests.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
