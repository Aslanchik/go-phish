package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/pipeline"
	"github.com/aslanchik/go-phish/internal/report"
	"github.com/aslanchik/go-phish/internal/telemetry"
)

func main() {
	skipLLM := flag.Bool("skip-llm", false, "skip the LLM call and use a stub hypothesis (for testing without an API key)")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gophish [--skip-llm] <url>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Investigates a suspicious URL and prints a structured phishing report.")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	rawURL := flag.Arg(0)
	if err := validateURL(rawURL); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := run(context.Background(), rawURL, *skipLLM); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, rawURL string, skipLLM bool) error {
	var llmClient anthropic.Client
	if !skipLLM {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set (use --skip-llm to bypass)")
		}
		llmClient = anthropic.NewClient()
	}

	conn, err := db.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := db.RunMigrations(conn); err != nil {
		return err
	}

	inv, err := db.CreateInvestigation(ctx, conn, rawURL)
	if err != nil {
		return fmt.Errorf("create investigation: %w", err)
	}

	shutdown, initErr := telemetry.Init(ctx)
	if initErr != nil {
		log.Printf("warn: telemetry init failed: %v — continuing without traces", initErr)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutErr := shutdown(shutCtx); shutErr != nil {
			log.Printf("warn: telemetry shutdown: %v", shutErr)
		}
	}()

	tracer := otel.Tracer(telemetry.TracerName)
	ctx, rootSpan := tracer.Start(ctx, "ssspy.investigation",
		trace.WithAttributes(
			attribute.String(telemetry.AttrInvestigationID, inv.ID),
			attribute.String(telemetry.AttrTargetURL, normalizeURL(rawURL)),
			attribute.String(telemetry.AttrAgentName, "go-phish"),
			attribute.String(telemetry.AttrAgentVersion, telemetry.Version()),
		))
	defer func() {
		if r := recover(); r != nil {
			rootSpan.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
			rootSpan.End()
			panic(r)
		}
	}()

	if err := pipeline.Run(ctx, inv.ID, conn, &llmClient, skipLLM, nil); err != nil {
		rootSpan.SetStatus(codes.Error, err.Error())
		rootSpan.End()
		return err
	}
	rootSpan.SetStatus(codes.Ok, "")
	rootSpan.End()

	inv, err = db.GetInvestigation(ctx, conn, inv.ID)
	if err != nil {
		return fmt.Errorf("read investigation: %w", err)
	}
	report.Print(inv)
	return nil
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}

func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return strings.TrimRight(u.String(), "/")
}
