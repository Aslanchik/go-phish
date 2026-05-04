package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net/url"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
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

	var llmClient anthropic.Client
	if !*skipLLM {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Fprintln(os.Stderr, "error: ANTHROPIC_API_KEY environment variable is not set (use --skip-llm to bypass)")
			os.Exit(1)
		}
		llmClient = anthropic.NewClient()
	}

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

	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create investigation: %v\n", err)
		os.Exit(1)
	}

	fail := func(msg string, args ...any) {
		errMsg := fmt.Sprintf(msg, args...)
		fmt.Fprintln(os.Stderr, "error:", errMsg)
		_ = db.UpdateStatus(ctx, conn, inv.ID, "failed", errMsg)
		os.Exit(1)
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, "fetching", ""); err != nil {
		fmt.Fprintf(os.Stderr, "error: update status: %v\n", err)
		os.Exit(1)
	}

	result, err := fetcher.Run(ctx, rawURL)
	if err != nil {
		fail("fetch: %v", err)
	}

	if err := db.UpdateArtifacts(ctx, conn, inv.ID, result); err != nil {
		fail("store artifacts: %v", err)
	}
	inv.FinalURL.String = result.FinalURL
	inv.FinalURL.Valid = true

	if err := db.UpdateStatus(ctx, conn, inv.ID, "hypothesizing", ""); err != nil {
		fail("update status: %v", err)
	}

	var hyp hypothesis.Hypothesis
	if *skipLLM {
		hyp = hypothesis.Hypothesis{
			Brand:          "unknown (--skip-llm)",
			TargetedAction: "other",
			Confidence:     "low",
			Reasoning:      "LLM call skipped; no analysis performed.",
		}
	} else {
		screenshotBytes, err := base64.StdEncoding.DecodeString(result.Screenshot)
		if err != nil {
			fail("decode screenshot: %v", err)
		}
		hyp, err = hypothesis.Generate(ctx, &llmClient, screenshotBytes, result.RenderedDOM)
		if err != nil {
			fail("hypothesis: %v", err)
		}
	}

	if err := db.UpdateHypothesis(ctx, conn, inv.ID, hyp); err != nil {
		fail("store hypothesis: %v", err)
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, "complete", ""); err != nil {
		fail("update status: %v", err)
	}

	// Re-read the updated investigation row so the report has all fields.
	inv, err = db.GetInvestigation(ctx, conn, inv.ID)
	if err != nil {
		fail("read investigation: %v", err)
	}

	reportText := report.Format(inv)
	if err := db.UpdateReport(ctx, conn, inv.ID, reportText); err != nil {
		fail("store report: %v", err)
	}

	report.Print(inv)
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
