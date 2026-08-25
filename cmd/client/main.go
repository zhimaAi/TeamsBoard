package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"goteams-client/internal/applog"
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
	if rawParentPID := strings.TrimSpace(os.Getenv("GOTEAMS_PARENT_PID")); rawParentPID != "" {
		parentPID, err := strconv.Atoi(rawParentPID)
		if err != nil || parentPID <= 0 {
			fmt.Fprintf(os.Stderr, "Invalid GOTEAMS_PARENT_PID: %q\n", rawParentPID)
			os.Exit(1)
		}
		go bootstrap.MonitorParentProcess(ctx, parentPID, cancel)
	}

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
		applog.Error("客户端启动失败", "error", err)
		fmt.Fprintf(os.Stderr, "Startup failed: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(ctx); err != nil && ctx.Err() == nil {
		applog.Error("客户端运行失败", "error", err)
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		os.Exit(1)
	}
}
