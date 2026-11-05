// Package database opens the Postgres connections Provider Platforms uses. It
// shares core's database.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DSN is APP_DSN if set, else built from the DB_* variables, as core does.
// DB_SSLMODE picks the TLS mode and defaults to prefer; core's DSN uses allow,
// which only encrypts if the server insists.
func DSN() string {
	if dsn := os.Getenv("APP_DSN"); dsn != "" {
		return dsn
	}
	sslmode := os.Getenv("DB_SSLMODE")
	if sslmode == "" {
		sslmode = "prefer"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), sslmode)
}

// Open is a plain connection as the database user, for the migrations.
func Open() (*sql.DB, error) {
	db, err := sql.Open("pgx", DSN())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// OpenAs is a pool whose sessions switch to the Postgres role named schema,
// which the first migration creates, and look up tables in schema first, then
// public. The role can write only that schema and read only core's users.
func OpenAs(schema string) (*gorm.DB, *sql.DB, error) {
	cfg, err := pgx.ParseConfig(DSN())
	if err != nil {
		return nil, nil, fmt.Errorf("parse the database connection string: %w", err)
	}
	cfg.RuntimeParams["search_path"] = schema + ", public"
	setRole := stdlib.OptionAfterConnect(func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{schema}.Sanitize())
		return err
	})
	sqlDB := stdlib.OpenDB(*cfg, setRole)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping database as %s: %w", schema, err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			LogLevel:                  logger.Warn,
			SlowThreshold:             200 * time.Millisecond,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("open gorm: %w", err)
	}
	return db, sqlDB, nil
}
