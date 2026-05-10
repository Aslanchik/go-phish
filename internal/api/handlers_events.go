package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// handleEvents — GET /api/v1/investigations/{id}/events
// Streams SSE events for the given investigation until terminal or disconnect.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	var afterSeq int64
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		afterSeq, _ = strconv.ParseInt(raw, 10, 64)
	}

	ch, cancel := s.broker.Subscribe(id, afterSeq)
	defer cancel()

	flusher, canFlush := w.(http.Flusher)

	for {
		select {
		case <-r.Context().Done():
			return
		case se, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(map[string]any{
				"investigation_id": se.Event.InvestigationID,
				"type":             se.Event.Type,
				"timestamp":        se.Event.Timestamp,
				"data":             se.Event.Data,
			})
			if err != nil {
				return
			}
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", se.Seq, data)
			if canFlush {
				flusher.Flush()
			}
			// Close stream after terminal events.
			if se.Event.Type == "complete" || se.Event.Type == "failed" {
				return
			}
		}
	}
}
