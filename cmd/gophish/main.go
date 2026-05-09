package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/agent"
	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
	"github.com/aslanchik/go-phish/internal/report"
	"github.com/aslanchik/go-phish/internal/synthesis"
	"github.com/aslanchik/go-phish/internal/tools"
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

	// fail updates the investigation to failed status and returns an error
	// so the caller can propagate it up and exit non-zero.
	fail := func(msg string, args ...any) error {
		err := fmt.Errorf(msg, args...)
		_ = db.UpdateStatus(ctx, conn, inv.ID, db.StatusFailed, err.Error())
		return err
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, db.StatusFetching, ""); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	log.Printf("phase 1: fetching %s", rawURL)
	result, err := fetcher.Run(ctx, rawURL)
	if err != nil {
		return fail("fetch: %w", err)
	}

	if err := db.UpdateArtifacts(ctx, conn, inv.ID, result); err != nil {
		return fail("store artifacts: %w", err)
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, db.StatusHypothesizing, ""); err != nil {
		return fail("update status: %w", err)
	}

	log.Printf("phase 2: generating hypothesis")
	var hyp hypothesis.Hypothesis
	if skipLLM {
		hyp = hypothesis.Hypothesis{
			Brand:          "unknown (--skip-llm)",
			TargetedAction: "other",
			Confidence:     "low",
			Reasoning:      "LLM call skipped; no analysis performed.",
		}
	} else {
		screenshotBytes, err := base64.StdEncoding.DecodeString(result.Screenshot)
		if err != nil {
			return fail("decode screenshot: %w", err)
		}
		hyp, err = hypothesis.Generate(ctx, &llmClient, screenshotBytes, result.RenderedDOM)
		if err != nil {
			return fail("hypothesis: %w", err)
		}
	}

	if err := db.UpdateHypothesis(ctx, conn, inv.ID, hyp); err != nil {
		return fail("store hypothesis: %w", err)
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, db.StatusEnriching, ""); err != nil {
		return fail("update status: %w", err)
	}

	log.Printf("phase 3: starting enrichment agent loop")
	inv, err = db.GetInvestigation(ctx, conn, inv.ID)
	if err != nil {
		return fail("read investigation before enrichment: %w", err)
	}

	if !skipLLM {
		toolServer, err := tools.New(ctx, &llmClient)
		if err != nil {
			return fail("start tool server: %w", err)
		}
		defer toolServer.Stop()

		enrichTrace, enrichSummary, err := agent.Run(ctx, inv, &llmClient, toolServer.Client)
		if err != nil {
			return fail("enrichment: %w", err)
		}
		log.Printf("phase 3: complete — %d tool calls", len(enrichTrace))

		traceJSON, err := json.Marshal(enrichTrace)
		if err != nil {
			return fail("marshal enrichment trace: %w", err)
		}
		if err := db.UpdateEnrichment(ctx, conn, inv.ID, traceJSON, enrichSummary); err != nil {
			return fail("store enrichment: %w", err)
		}
	}

	log.Printf("phase 4: synthesising findings")
	if err := db.UpdateStatus(ctx, conn, inv.ID, db.StatusSynthesizing, ""); err != nil {
		return fail("update status: %w", err)
	}

	inv, err = db.GetInvestigation(ctx, conn, inv.ID)
	if err != nil {
		return fail("read investigation before synthesis: %w", err)
	}

	var synthResult synthesis.Result
	if skipLLM {
		skipped := synthesis.Claim{Confidence: "low", Evidence: "LLM call skipped; no analysis performed"}
		synthResult = synthesis.Result{
			BrandImpersonated:   skipped,
			KitIdentification:   skipped,
			ExfilTarget:         skipped,
			InfrastructureNotes: skipped,
			Verdict:             synthesis.Claim{Value: "inconclusive", Confidence: "low", Evidence: "LLM call skipped; no analysis performed"},
		}
	} else {
		synthResult, err = synthesis.Generate(ctx, &llmClient, inv)
		if err != nil {
			return fail("synthesis: %w", err)
		}
	}

	synthJSON, err := json.Marshal(synthResult)
	if err != nil {
		return fail("marshal synthesis result: %w", err)
	}
	if err := db.UpdateSynthesis(ctx, conn, inv.ID, synthJSON); err != nil {
		return fail("store synthesis: %w", err)
	}

	if err := db.UpdateStatus(ctx, conn, inv.ID, db.StatusComplete, ""); err != nil {
		return fail("update status: %w", err)
	}

	inv, err = db.GetInvestigation(ctx, conn, inv.ID)
	if err != nil {
		return fail("read investigation: %w", err)
	}

	reportText := report.Format(inv)
	if err := db.UpdateReport(ctx, conn, inv.ID, reportText); err != nil {
		return fail("store report: %w", err)
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
