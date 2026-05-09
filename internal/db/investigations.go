package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
)

type Investigation struct {
	ID           string
	URL          string
	CreatedAt    time.Time
	Status       string
	ErrorMessage sql.NullString
	FinalURL     sql.NullString
	RenderedDOM  sql.NullString
	Screenshot   []byte
	NetworkLog   json.RawMessage
	JSFiles      json.RawMessage
	Forms        json.RawMessage
	Hypothesis        json.RawMessage
	Report            sql.NullString
	EnrichmentTrace   json.RawMessage
	EnrichmentSummary sql.NullString
}

func CreateInvestigation(ctx context.Context, conn *sql.DB, url string) (Investigation, error) {
	var inv Investigation
	err := conn.QueryRowContext(ctx, `
		INSERT INTO investigations (url)
		VALUES ($1)
		RETURNING id, url, created_at, status
	`, url).Scan(&inv.ID, &inv.URL, &inv.CreatedAt, &inv.Status)
	if err != nil {
		return Investigation{}, fmt.Errorf("create investigation: %w", err)
	}
	return inv, nil
}

func UpdateStatus(ctx context.Context, conn *sql.DB, id string, status Status, errMsg string) error {
	var nullErr sql.NullString
	if errMsg != "" {
		nullErr = sql.NullString{String: errMsg, Valid: true}
	}
	_, err := conn.ExecContext(ctx, `
		UPDATE investigations SET status = $1, error_message = $2 WHERE id = $3
	`, status, nullErr, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func UpdateArtifacts(ctx context.Context, conn *sql.DB, id string, artifacts fetcher.FetchResult) error {
	screenshot, err := base64.StdEncoding.DecodeString(artifacts.Screenshot)
	if err != nil {
		return fmt.Errorf("decode screenshot: %w", err)
	}
	networkLog, err := json.Marshal(artifacts.NetworkLog)
	if err != nil {
		return fmt.Errorf("marshal network_log: %w", err)
	}
	jsFiles, err := json.Marshal(artifacts.JSFiles)
	if err != nil {
		return fmt.Errorf("marshal js_files: %w", err)
	}
	forms, err := json.Marshal(artifacts.Forms)
	if err != nil {
		return fmt.Errorf("marshal forms: %w", err)
	}
	_, err = conn.ExecContext(ctx, `
		UPDATE investigations
		SET final_url = $1, rendered_dom = $2, screenshot = $3,
		    network_log = $4, js_files = $5, forms = $6
		WHERE id = $7
	`, artifacts.FinalURL, artifacts.RenderedDOM, screenshot,
		networkLog, jsFiles, forms, id)
	if err != nil {
		return fmt.Errorf("update artifacts: %w", err)
	}
	return nil
}

func UpdateHypothesis(ctx context.Context, conn *sql.DB, id string, h hypothesis.Hypothesis) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal hypothesis: %w", err)
	}
	_, err = conn.ExecContext(ctx, `
		UPDATE investigations SET hypothesis = $1 WHERE id = $2
	`, data, id)
	if err != nil {
		return fmt.Errorf("update hypothesis: %w", err)
	}
	return nil
}

func GetInvestigation(ctx context.Context, conn *sql.DB, id string) (Investigation, error) {
	var inv Investigation
	var enrichmentTrace []byte
	err := conn.QueryRowContext(ctx, `
		SELECT id, url, created_at, status, error_message,
		       final_url, rendered_dom, screenshot,
		       network_log, js_files, forms, hypothesis, report,
		       enrichment_trace, enrichment_summary
		FROM investigations WHERE id = $1
	`, id).Scan(
		&inv.ID, &inv.URL, &inv.CreatedAt, &inv.Status, &inv.ErrorMessage,
		&inv.FinalURL, &inv.RenderedDOM, &inv.Screenshot,
		&inv.NetworkLog, &inv.JSFiles, &inv.Forms, &inv.Hypothesis, &inv.Report,
		&enrichmentTrace, &inv.EnrichmentSummary,
	)
	if err != nil {
		return Investigation{}, fmt.Errorf("get investigation: %w", err)
	}
	inv.EnrichmentTrace = json.RawMessage(enrichmentTrace)
	return inv, nil
}

func UpdateEnrichment(ctx context.Context, conn *sql.DB, id string, trace json.RawMessage, summary string) error {
	_, err := conn.ExecContext(ctx, `
		UPDATE investigations SET enrichment_trace = $1, enrichment_summary = $2 WHERE id = $3
	`, trace, summary, id)
	if err != nil {
		return fmt.Errorf("update enrichment: %w", err)
	}
	return nil
}

func UpdateReport(ctx context.Context, conn *sql.DB, id, report string) error {
	_, err := conn.ExecContext(ctx, `
		UPDATE investigations SET report = $1 WHERE id = $2
	`, report, id)
	if err != nil {
		return fmt.Errorf("update report: %w", err)
	}
	return nil
}
