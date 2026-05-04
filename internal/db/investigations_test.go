package db_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"testing"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := db.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestCreateInvestigation(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	if inv.ID == "" {
		t.Error("expected non-empty ID")
	}
	if inv.URL != "https://example.com/phish" {
		t.Errorf("got URL %q, want %q", inv.URL, "https://example.com/phish")
	}
	if inv.Status != "pending" {
		t.Errorf("got status %q, want %q", inv.Status, "pending")
	}
	if inv.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestUpdateStatus(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	if err := db.UpdateStatus(ctx, conn, inv.ID, "fetching", ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	var status string
	var errMsg sql.NullString
	conn.QueryRowContext(ctx, "SELECT status, error_message FROM investigations WHERE id = $1", inv.ID).
		Scan(&status, &errMsg)
	if status != "fetching" {
		t.Errorf("got status %q, want %q", status, "fetching")
	}
	if errMsg.Valid {
		t.Errorf("expected NULL error_message, got %q", errMsg.String)
	}
}

func TestUpdateStatus_WithError(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	if err := db.UpdateStatus(ctx, conn, inv.ID, "failed", "container exited 1"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	var status string
	var errMsg sql.NullString
	conn.QueryRowContext(ctx, "SELECT status, error_message FROM investigations WHERE id = $1", inv.ID).
		Scan(&status, &errMsg)
	if status != "failed" {
		t.Errorf("got status %q, want %q", status, "failed")
	}
	if !errMsg.Valid || errMsg.String != "container exited 1" {
		t.Errorf("got error_message %v, want %q", errMsg, "container exited 1")
	}
}

func TestUpdateStatus_UpdatesInPlace(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	db.UpdateStatus(ctx, conn, inv.ID, "fetching", "")
	db.UpdateStatus(ctx, conn, inv.ID, "complete", "")

	var count int
	conn.QueryRowContext(ctx, "SELECT count(*) FROM investigations WHERE id = $1", inv.ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}

	var status string
	conn.QueryRowContext(ctx, "SELECT status FROM investigations WHERE id = $1", inv.ID).Scan(&status)
	if status != "complete" {
		t.Errorf("got status %q, want %q", status, "complete")
	}
}

func TestUpdateArtifacts(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	fakeScreenshot := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	result := fetcher.FetchResult{
		FinalURL:    "https://example.com/phish/login",
		RenderedDOM: "<html><body>Login</body></html>",
		Screenshot:  fakeScreenshot,
		NetworkLog:  []fetcher.NetworkEntry{{URL: "https://example.com/phish", Method: "GET", Status: 200}},
		JSFiles:     []fetcher.JSFile{{URL: "https://example.com/app.js", Content: "var x=1;"}},
		Forms:       []fetcher.Form{{Action: "/submit", Method: "POST", Fields: []fetcher.FormField{{Name: "email", Type: "text"}}}},
	}

	if err := db.UpdateArtifacts(ctx, conn, inv.ID, result); err != nil {
		t.Fatalf("UpdateArtifacts: %v", err)
	}

	var finalURL sql.NullString
	var renderedDOM sql.NullString
	var screenshot []byte
	conn.QueryRowContext(ctx, "SELECT final_url, rendered_dom, screenshot FROM investigations WHERE id = $1", inv.ID).
		Scan(&finalURL, &renderedDOM, &screenshot)

	if finalURL.String != result.FinalURL {
		t.Errorf("got final_url %q, want %q", finalURL.String, result.FinalURL)
	}
	if renderedDOM.String != result.RenderedDOM {
		t.Errorf("got rendered_dom %q, want %q", renderedDOM.String, result.RenderedDOM)
	}
	if string(screenshot) != "fake-png-bytes" {
		t.Errorf("screenshot bytes mismatch")
	}
}

func TestUpdateHypothesis(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	h := hypothesis.Hypothesis{
		Brand:          "PayPal",
		TargetedAction: "credential_theft",
		Confidence:     "high",
		Reasoning:      "Login form matches PayPal branding with credential harvesting action.",
	}
	if err := db.UpdateHypothesis(ctx, conn, inv.ID, h); err != nil {
		t.Fatalf("UpdateHypothesis: %v", err)
	}

	var brand string
	conn.QueryRowContext(ctx, "SELECT hypothesis->>'brand' FROM investigations WHERE id = $1", inv.ID).Scan(&brand)
	if brand != "PayPal" {
		t.Errorf("got brand %q, want %q", brand, "PayPal")
	}
}

func TestUpdateReport(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	inv, err := db.CreateInvestigation(ctx, conn, "https://example.com/phish")
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM investigations WHERE id = $1", inv.ID)
	})

	report := "Investigation complete. Brand: PayPal. Verdict: phishing."
	if err := db.UpdateReport(ctx, conn, inv.ID, report); err != nil {
		t.Fatalf("UpdateReport: %v", err)
	}

	var stored sql.NullString
	conn.QueryRowContext(ctx, "SELECT report FROM investigations WHERE id = $1", inv.ID).Scan(&stored)
	if stored.String != report {
		t.Errorf("got report %q, want %q", stored.String, report)
	}
}
