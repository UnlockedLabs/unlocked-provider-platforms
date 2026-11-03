// Command app is Provider Platforms' one binary: "app serve" runs the API and
// the job scheduler, "app worker" runs the LMS jobs, and "app migrate"
// applies the database migrations. Serve and worker stop cleanly on SIGINT or
// SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/app"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/server"
)

const usage = "usage: app serve | app worker | app migrate"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	initLogging()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "serve":
		return app.Serve(ctx, app.ServeConfig{
			Addr:       envOr("ADDR", ":8080"),
			PathPrefix: os.Getenv("PATH_PREFIX"),
			Version:    server.VersionFromEnv(),
			Token:      os.Getenv("SERVICE_TOKEN"),
			CoreURL:    os.Getenv("CORE_URL"),
		})
	case "worker":
		return app.Work(ctx)
	case "migrate":
		return app.Migrate(ctx)
	default:
		return fmt.Errorf("unknown command %q: %s", args[0], usage)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
