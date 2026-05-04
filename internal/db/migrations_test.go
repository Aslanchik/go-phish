package db_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/aslanchik/go-phish/internal/db"
)

func TestRunMigrations(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify the table exists with expected columns
	var count int
	err = conn.QueryRow(`
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'investigations'
	`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("investigations table has no columns — migration did not apply")
	}
	t.Logf("investigations table has %d columns", count)
}
