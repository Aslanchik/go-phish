package main

import (
	"fmt"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/db"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "error: ANTHROPIC_API_KEY environment variable is not set")
		os.Exit(1)
	}
	_ = anthropic.NewClient() // validated key is present; full use wired in T-12

	conn, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := db.RunMigrations(conn); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
