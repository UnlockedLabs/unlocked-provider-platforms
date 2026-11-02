package migrate

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	_ "github.com/glebarez/go-sqlite"
	"github.com/pressly/goose/v3"
)

func open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var migrations = fstest.MapFS{
	"migrations/00001_one.sql": {Data: []byte("-- +goose Up\nCREATE TABLE one (id integer);\n-- +goose Down\nDROP TABLE one;\n")},
	"migrations/00002_two.sql": {Data: []byte("-- +goose Up\nCREATE TABLE two (id integer);\n-- +goose Down\nDROP TABLE two;\n")},
}

func TestUp(t *testing.T) {
	ctx := context.Background()
	db := open(t)

	pending, err := Pending(ctx, db, goose.DialectSQLite3, migrations)
	if err != nil || !pending {
		t.Fatalf("Pending() before = %v, %v, want true (a missing ledger is pending)", pending, err)
	}
	for run := 1; run <= 2; run++ {
		if err := Up(ctx, db, goose.DialectSQLite3, migrations); err != nil {
			t.Fatalf("Up() run %d error = %v", run, err)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM goose_db_version_providerplatforms WHERE version_id > 0`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("ledger goose_db_version_providerplatforms has %d versions, %v, want 2 applied once each", n, err)
	}
	if pending, err = Pending(ctx, db, goose.DialectSQLite3, migrations); err != nil || pending {
		t.Fatalf("Pending() after = %v, %v, want false", pending, err)
	}
}

func TestPendingSeesANewMigration(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	if err := Up(ctx, db, goose.DialectSQLite3, migrations); err != nil {
		t.Fatal(err)
	}
	more := fstest.MapFS{"migrations/00003_three.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	for k, v := range migrations {
		more[k] = v
	}
	if pending, err := Pending(ctx, db, goose.DialectSQLite3, more); err != nil || !pending {
		t.Fatalf("Pending() = %v, %v, want true", pending, err)
	}
}
