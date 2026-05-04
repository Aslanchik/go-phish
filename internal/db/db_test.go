package db_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/aslanchik/go-phish/internal/db"
)

func TestOpen_MissingEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := db.Open()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is unset, got nil")
	}
	want := "DATABASE_URL environment variable is not set"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestOpen_Unreachable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://gophish:gophish@localhost:9999/gophish?sslmode=disable&connect_timeout=2")
	_, err := db.Open()
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
}

func TestOpen_HappyPath(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := db.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		t.Fatalf("ping after Open: %v", err)
	}
}

func TestRunMigrations_BrokenConn(t *testing.T) {
	// Open a valid connection then close it to simulate a broken DB handle.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close() // deliberately closed before RunMigrations
	if err := db.RunMigrations(conn); err == nil {
		t.Fatal("expected error running migrations on closed connection, got nil")
	}
}
