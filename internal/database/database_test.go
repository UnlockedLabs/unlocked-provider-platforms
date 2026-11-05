package database

import (
	"strings"
	"testing"
)

func TestDSNSSLMode(t *testing.T) {
	t.Setenv("APP_DSN", "")
	t.Setenv("DB_HOST", "db")
	t.Run("defaults to prefer", func(t *testing.T) {
		t.Setenv("DB_SSLMODE", "")
		if got := DSN(); !strings.Contains(got, "sslmode=prefer") {
			t.Fatalf("DSN() = %q, want sslmode=prefer", got)
		}
	})
	t.Run("DB_SSLMODE overrides it", func(t *testing.T) {
		t.Setenv("DB_SSLMODE", "require")
		if got := DSN(); !strings.Contains(got, "sslmode=require") {
			t.Fatalf("DSN() = %q, want sslmode=require", got)
		}
	})
}
