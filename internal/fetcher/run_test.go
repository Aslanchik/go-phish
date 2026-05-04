package fetcher_test

import (
	"context"
	"os"
	"testing"

	"github.com/aslanchik/go-phish/internal/fetcher"
)

func TestRun(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run fetcher integration tests (requires Docker + go-phish-fetcher image)")
	}
	ctx := context.Background()
	result, err := fetcher.Run(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalURL == "" {
		t.Error("FinalURL is empty")
	}
	if result.RenderedDOM == "" {
		t.Error("RenderedDOM is empty")
	}
	if result.Screenshot == "" {
		t.Error("Screenshot is empty")
	}
	if result.NetworkLog == nil {
		t.Error("NetworkLog is nil")
	}
	t.Logf("final_url=%s dom_len=%d screenshot_len=%d net_entries=%d",
		result.FinalURL, len(result.RenderedDOM), len(result.Screenshot), len(result.NetworkLog))
}

func TestRun_InvalidURL(t *testing.T) {
	_, err := fetcher.Run(context.Background(), "://bad")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
