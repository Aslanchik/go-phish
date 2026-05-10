package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"flag"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/pipeline"
	"github.com/aslanchik/go-phish/internal/report"
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

	if err := pipeline.Run(ctx, inv.ID, conn, &llmClient, skipLLM, nil); err != nil {
		return err
	}

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
