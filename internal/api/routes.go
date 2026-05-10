package api

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /api/v1/investigations", s.handleSubmit)
	s.mux.HandleFunc("GET /api/v1/investigations", s.handleList)
	// More specific patterns must be registered before the wildcard.
	// Go 1.22 ServeMux matches longest path, so screenshot beats {id} automatically,
	// but explicit ordering here keeps intent clear.
	s.mux.HandleFunc("GET /api/v1/investigations/{id}/screenshot", s.handleScreenshot)
	s.mux.HandleFunc("GET /api/v1/investigations/{id}/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/v1/investigations/{id}", s.handleGet)
	// SPA fallback — serves index.html for all unmatched paths (implemented in T16).
	s.mux.HandleFunc("/", s.handleSPA)
}
