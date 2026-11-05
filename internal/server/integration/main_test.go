// Package integration runs core's provider-route scenarios (EN-193) against
// this service's HTTP handlers over a real Postgres: core's schema, then this
// service's migrations. It needs core's migrations directory and a database:
//
//	CORE_MIGRATIONS=/path/to/unlocked-core/backend/migrations go test ./internal/server/integration
//
// With PROVIDER_TEST_DSN (a key=value DSN without dbname, for a server where
// it may create databases) it uses that server, otherwise it starts
// postgres:16 in Docker. Without either it skips.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/migrate"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/providerplatforms"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const dbName = "provider_integration"

// skipReason is why the tests can't run, or empty when they can.
var skipReason string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	coreMigrations := os.Getenv("CORE_MIGRATIONS")
	if coreMigrations == "" {
		skipReason = "CORE_MIGRATIONS (core's backend/migrations directory) is not set"
		return m.Run()
	}
	admin, stop, err := adminDSN()
	if err != nil {
		skipReason = err.Error()
		return m.Run()
	}
	defer stop()
	// The image runs in UTC. Dates the service scans come back in the process's
	// zone, so a developer's own zone would change the golden files.
	time.Local = time.UTC
	dsn, err := freshDatabase(admin, coreMigrations)
	if err != nil {
		fmt.Fprintln(os.Stderr, "set up database:", err)
		return 1
	}
	if err := os.Setenv("APP_DSN", dsn); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

// adminDSN is a DSN for a server the tests may create a database on, and a
// function that stops it.
func adminDSN() (string, func(), error) {
	if dsn := os.Getenv("PROVIDER_TEST_DSN"); dsn != "" {
		return dsn, func() {}, nil
	}
	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::5432",
		"-e", "POSTGRES_PASSWORD=test", "postgres:16").Output()
	if err != nil {
		return "", nil, fmt.Errorf("PROVIDER_TEST_DSN is not set and docker could not start postgres: %w", err)
	}
	id := strings.TrimSpace(string(out))
	stop := func() { _ = exec.Command("docker", "rm", "-f", id).Run() }
	port, err := exec.Command("docker", "port", id, "5432/tcp").Output()
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("find the postgres port: %w", err)
	}
	hostPort := strings.TrimSpace(strings.SplitN(string(port), "\n", 2)[0])
	i := strings.LastIndex(hostPort, ":")
	dsn := fmt.Sprintf("host=%s port=%s user=postgres password=test sslmode=disable", hostPort[:i], hostPort[i+1:])
	if err := waitFor(dsn); err != nil {
		stop()
		return "", nil, err
	}
	return dsn, stop, nil
}

func waitFor(dsn string) error {
	var err error
	for range 60 {
		var db *sql.DB
		if db, err = sql.Open("pgx", dsn); err == nil {
			err = db.Ping()
			_ = db.Close()
			if err == nil {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("postgres did not become ready: %w", err)
}

// freshDatabase creates an empty database on the admin server, applies core's
// migrations and then this service's (the order a deploy runs them in), and
// returns the new database's DSN.
func freshDatabase(admin, coreMigrations string) (string, error) {
	ctx := context.Background()
	db, err := sql.Open("pgx", admin)
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+dbName); err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+dbName); err != nil {
		return "", err
	}
	dsn := admin + " dbname=" + dbName
	fresh, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", err
	}
	defer func() { _ = fresh.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, fresh, os.DirFS(coreMigrations),
		goose.WithTableName("public.goose_db_version"))
	if err != nil {
		return "", fmt.Errorf("core migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return "", fmt.Errorf("apply core migrations: %w", err)
	}
	if err := migrate.Up(ctx, fresh, goose.DialectPostgres, providerplatforms.Module{}.Migrations()); err != nil {
		return "", fmt.Errorf("apply provider migrations: %w", err)
	}
	return dsn, nil
}
