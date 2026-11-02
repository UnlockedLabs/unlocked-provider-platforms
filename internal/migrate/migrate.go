// Package migrate runs Provider Platforms' embedded migrations against its own
// goose ledger.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// Ledger is the goose version table. It is the one core's migration runner
// used for this module before the move (goose_db_version_providerplatforms),
// so a database migrated by core isn't migrated again.
const Ledger = "goose_db_version_providerplatforms"

// Up applies the migrations in fsys's "migrations" folder that the ledger
// doesn't record yet. They move tables core created out of public, so core's
// migrations have to run first.
func Up(ctx context.Context, db *sql.DB, dialect goose.Dialect, fsys fs.FS) error {
	p, err := provider(db, dialect, fsys)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Pending reports whether fsys has a migration the ledger doesn't record as
// applied. It only reads, so a process that doesn't migrate can use it to wait
// for the one that does. A ledger that doesn't exist yet counts as pending.
func Pending(ctx context.Context, db *sql.DB, dialect goose.Dialect, fsys fs.FS) (bool, error) {
	p, err := provider(db, dialect, fsys)
	if err != nil {
		return false, err
	}
	var applied int64
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM `+Ledger+` WHERE is_applied`).Scan(&applied)
	if err != nil {
		// The ledger doesn't exist until the first run.
		return true, nil
	}
	sources := p.ListSources()
	return applied < sources[len(sources)-1].Version, nil
}

func provider(db *sql.DB, dialect goose.Dialect, fsys fs.FS) (*goose.Provider, error) {
	dir, err := fs.Sub(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrations folder: %w", err)
	}
	p, err := goose.NewProvider(dialect, db, dir,
		goose.WithTableName(Ledger),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return p, nil
}
