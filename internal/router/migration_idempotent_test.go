package router_test

import (
	"cc-052/internal/repository"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestMigrationsIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := repository.RunMigrations(db, "../../migrations"); err != nil {
			t.Fatalf("migration run %d failed: %v", i+1, err)
		}
	}
}
