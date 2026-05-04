package main

import (
	"fmt"
	"os"

	"github.com/aslanchik/go-phish/internal/db"
)

func main() {
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
