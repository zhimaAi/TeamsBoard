package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"goteams-client/internal/bootstrap"
)

// Injected by Taskfile through -ldflags -X during build to facilitate tracking of product version.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle system signals: the first signal triggers a graceful exit, and the second signal forces the process to end.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal, shutting down...")
		cancel()
		// Wait for the second signal: if it still cannot exit, force the end directly to avoid having to kill -9.
		<-sigCh
		fmt.Println("Received shutdown signal again, forcing exit.")
		os.Exit(1)
	}()

	// Start application
	app, err := bootstrap.New(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Startup failed: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		os.Exit(1)
	}
}
