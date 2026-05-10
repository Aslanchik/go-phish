package api

import (
	"database/sql"
	"net/http"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// Server holds shared dependencies and the configured mux.
type Server struct {
	db     *sql.DB
	llm    *anthropic.Client
	broker *Broker
	mux    *http.ServeMux
}

// New wires up all dependencies and registers routes.
func New(db *sql.DB, llm *anthropic.Client) *Server {
	s := &Server{
		db:     db,
		llm:    llm,
		broker: newBroker(),
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the http.Handler for use with http.ListenAndServe.
func (s *Server) Handler() http.Handler {
	return s.mux
}
