// Package app wires Provider Platforms' three commands: migrate, serve and
// worker.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/UnlockedLabs/unlocked-contracts/coreapi"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/database"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/migrate"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/providerplatforms"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/scheduler"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/server"

	"github.com/nats-io/nats.go"
	"github.com/pressly/goose/v3"
)

// migrationPollInterval is how often serve and worker check whether the
// database has caught up with this image's migrations.
const migrationPollInterval = 5 * time.Second

// ServeConfig is what serve needs.
type ServeConfig struct {
	Addr       string
	PathPrefix string
	Version    server.Version
	// Token is the shared service token every internal call carries.
	Token string
	// CoreURL is where core's internal /internal/v1/core/ endpoints are.
	CoreURL string
}

// Migrate applies the embedded migrations to the database.
func Migrate(ctx context.Context) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := migrate.Up(ctx, db, goose.DialectPostgres, providerplatforms.Module{}.Migrations()); err != nil {
		return err
	}
	slog.Info("migrated", "ledger", migrate.Ledger)
	return nil
}

// Serve runs the API and the job scheduler until ctx is canceled. Core
// proxies the module's routes here and calls the typed interface under
// /internal/v1/provider-platforms/.
func Serve(ctx context.Context, cfg ServeConfig) error {
	if cfg.Token == "" {
		return errors.New("SERVICE_TOKEN is not set")
	}
	if cfg.CoreURL == "" {
		return errors.New("CORE_URL is not set")
	}
	deps, closeAll, err := connect(ctx)
	if err != nil {
		return err
	}
	defer closeAll()
	deps.Users = coreapi.NewUsersClient(cfg.CoreURL, cfg.Token, &http.Client{})
	deps.LiveProviders = coreapi.NewLiveProvidersClient(cfg.CoreURL, cfg.Token, &http.Client{})

	var routes routeList
	providerplatforms.Module{}.Register(&routes, deps)

	sched, err := scheduler.New()
	if err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer func() {
		if err := sched.Stop(); err != nil {
			slog.Error("stop scheduler", "err", err)
		}
	}()
	providerplatforms.Module{}.RegisterJobs(sched, deps)

	return server.Serve(ctx, cfg.Addr, cfg.PathPrefix, cfg.Version,
		server.Routes(cfg.Token, routes),
		server.Internal(cfg.Token, providerplatforms.NewAPI(deps.DB)),
	)
}

// Work runs the LMS jobs and serves the provider user lists until ctx is
// canceled.
func Work(ctx context.Context) error {
	deps, closeAll, err := connect(ctx)
	if err != nil {
		return err
	}
	defer closeAll()
	slog.Info("worker started")
	defer slog.Info("worker stopped")
	return providerplatforms.Module{}.Work(ctx, deps)
}

type routeList []module.Route

func (r *routeList) Handle(rt module.Route) { *r = append(*r, rt) }

// connect waits until the database is migrated, then opens the module's
// connection and NATS.
func connect(ctx context.Context) (module.Deps, func(), error) {
	plain, err := database.Open()
	if err != nil {
		return module.Deps{}, nil, fmt.Errorf("open database: %w", err)
	}
	err = waitMigrated(ctx, plain)
	_ = plain.Close()
	if err != nil {
		return module.Deps{}, nil, err
	}
	db, sqlDB, err := database.OpenAs(providerplatforms.Module{}.Schema())
	if err != nil {
		return module.Deps{}, nil, err
	}
	nc, err := connectNATS()
	if err != nil {
		_ = sqlDB.Close()
		return module.Deps{}, nil, err
	}
	return module.Deps{DB: db, NATS: nc}, func() { nc.Close(); _ = sqlDB.Close() }, nil
}

// waitMigrated blocks until the database has every migration this image ships
// with, because starting earlier would query tables that aren't there yet.
func waitMigrated(ctx context.Context, db *sql.DB) error {
	for {
		pending, err := migrate.Pending(ctx, db, goose.DialectPostgres, providerplatforms.Module{}.Migrations())
		switch {
		case err != nil:
			slog.Warn("waiting for migrations", "err", err)
		case pending:
			slog.Info("waiting for migrations to finish")
		default:
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(migrationPollInterval):
		}
	}
}

// connectNATS connects with NATS_URL, NATS_USER and NATS_PASSWORD, as core does.
func connectNATS() (*nats.Conn, error) {
	options := nats.GetDefaultOptions()
	options.Url = os.Getenv("NATS_URL")
	options.User = os.Getenv("NATS_USER")
	options.Password = os.Getenv("NATS_PASSWORD")
	nc, err := options.Connect()
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}
	return nc, nil
}
