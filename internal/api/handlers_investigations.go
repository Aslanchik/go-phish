package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/pipeline"
	"github.com/aslanchik/go-phish/internal/synthesis"
)

// handleSubmit — POST /api/v1/investigations
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		clientError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := validateURL(body.URL); err != nil {
		clientError(w, http.StatusBadRequest, err.Error())
		return
	}

	inv, err := db.CreateInvestigation(r.Context(), s.db, body.URL)
	if err != nil {
		safeError(w, err)
		return
	}

	go func() {
		_ = pipeline.Run(
			context.Background(),
			inv.ID,
			s.db,
			s.llm,
			false,
			s.broker.Publish,
		)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     inv.ID,
		"status": inv.Status,
	})
}

// handleList — GET /api/v1/investigations
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	invs, err := db.ListInvestigations(r.Context(), s.db)
	if err != nil {
		safeError(w, err)
		return
	}

	type item struct {
		ID        string  `json:"id"`
		URL       string  `json:"url"`
		Status    string  `json:"status"`
		CreatedAt time.Time `json:"created_at"`
		Verdict   *string `json:"verdict,omitempty"`
	}

	out := make([]item, 0, len(invs))
	for _, inv := range invs {
		it := item{
			ID:        inv.ID,
			URL:       inv.URL,
			Status:    inv.Status,
			CreatedAt: inv.CreatedAt,
		}
		if len(inv.Synthesis) > 0 {
			var s synthesis.Result
			if json.Unmarshal(inv.Synthesis, &s) == nil && s.Verdict.Value != "" {
				it.Verdict = &s.Verdict.Value
			}
		}
		out = append(out, it)
	}

	writeJSON(w, http.StatusOK, out)
}

// handleGet — GET /api/v1/investigations/{id}
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := db.GetInvestigation(r.Context(), s.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clientError(w, http.StatusNotFound, "not found")
			return
		}
		safeError(w, err)
		return
	}

	type response struct {
		ID                string          `json:"id"`
		URL               string          `json:"url"`
		CreatedAt         time.Time       `json:"created_at"`
		Status            string          `json:"status"`
		ErrorMessage      *string         `json:"error_message,omitempty"`
		Hypothesis        json.RawMessage `json:"hypothesis,omitempty"`
		EnrichmentSummary *string         `json:"enrichment_summary,omitempty"`
		Synthesis         json.RawMessage `json:"synthesis,omitempty"`
	}

	resp := response{
		ID:        inv.ID,
		URL:       inv.URL,
		CreatedAt: inv.CreatedAt,
		Status:    inv.Status,
	}
	if inv.ErrorMessage.Valid {
		resp.ErrorMessage = &inv.ErrorMessage.String
	}
	if len(inv.Hypothesis) > 0 {
		resp.Hypothesis = inv.Hypothesis
	}
	if inv.EnrichmentSummary.Valid {
		resp.EnrichmentSummary = &inv.EnrichmentSummary.String
	}
	if len(inv.Synthesis) > 0 {
		resp.Synthesis = inv.Synthesis
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleScreenshot — GET /api/v1/investigations/{id}/screenshot
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := db.GetInvestigation(r.Context(), s.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clientError(w, http.StatusNotFound, "not found")
			return
		}
		safeError(w, err)
		return
	}

	if len(inv.Screenshot) == 0 {
		clientError(w, http.StatusNotFound, "screenshot unavailable")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(inv.Screenshot)
}

func validateURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("url has no host")
	}
	return nil
}
