package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/api"
	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/telemetry"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	ctx := context.Background()

	shutdown, err := telemetry.Init(ctx)
	if err != nil {
		log.Printf("warn: telemetry init failed: %v — continuing without traces", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutErr := shutdown(shutCtx); shutErr != nil {
			log.Printf("warn: telemetry shutdown: %v", shutErr)
		}
	}()

	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()

	if err := db.RunMigrations(conn); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	var llmClient anthropic.Client
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Printf("warning: ANTHROPIC_API_KEY not set — LLM calls will fail at investigation time")
	} else {
		llmClient = anthropic.NewClient()
	}

	srv := api.New(conn, &llmClient)
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
