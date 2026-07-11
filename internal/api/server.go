package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/florian-roos/suffren/internal/limiter"
)

type Request struct {
	Identifier     string `json:"identifier"`
	Resource       string `json:"resource"`
	Limit          uint64 `json:"limit"`
	Window         string `json:"window"`
	ValueRequested uint64 `json:"value_requested"`
}

type CheckResponse struct {
	Allowed   bool   `json:"allowed"`
	Current   uint64 `json:"current"`
	Limit     uint64 `json:"limit"`
	Remaining uint64 `json:"remaining"`
	ResetAt   string `json:"reset_at"`
}

type StatusResponse struct {
	Current   uint64 `json:"current"`
	Limit     uint64 `json:"limit"`
	Remaining uint64 `json:"remaining"`
	ResetAt   string `json:"reset_at"`
}

type Server struct {
	router  *http.ServeMux
	limiter *limiter.Limiter
}

// RouterForTest returns the HTTP router for testing purposes.
func (s *Server) RouterForTest() http.Handler {
	return s.router
}

func NewServer(limiter *limiter.Limiter) *Server {
	s := &Server{
		router:  http.NewServeMux(),
		limiter: limiter,
	}

	s.router.HandleFunc("POST /check", s.handleCheck)
	s.router.HandleFunc("POST /status", s.handleStatus)

	return s
}

// starts the API HTTP server.
func (s *Server) Start(address string) error {
	slog.Info("API server started", "address", address)
	handler := LoggingMiddleware(s.router)
	return http.ListenAndServe(address, handler)
}

// handles the API call to check wether the request respects the limit rules
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	req, window, err := decodeRequest(w, r)
	if err != nil {
		return
	}

	decision := s.limiter.Check(req.Identifier, req.Resource, req.ValueRequested, limiter.Rule{Limit: req.Limit, Window: window})

	if decision.Error != nil {
		http.Error(w, "Timeout, network unreachable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := CheckResponse{
		Allowed:   decision.Allowed,
		Current:   decision.Current,
		Limit:     decision.Limit,
		Remaining: decision.Remaining,
		ResetAt:   decision.ResetAt.Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// handles the API call to return the status of an identifier given a resource and rule
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	req, window, err := decodeRequest(w, r)
	if err != nil {
		return
	}

	status := s.limiter.Status(req.Identifier, req.Resource, limiter.Rule{Limit: req.Limit, Window: window})

	if status.Error != nil {
		http.Error(w, "Ratelimite unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := StatusResponse{
		Current:   status.Current,
		Limit:     status.Limit,
		Remaining: status.Remaining,
		ResetAt:   status.ResetAt.Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func decodeRequest(w http.ResponseWriter, r *http.Request) (Request, time.Duration, error) {
	var req Request

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return Request{}, time.Duration(0), err
	}

	window, err := time.ParseDuration(req.Window)
	if err != nil {
		http.Error(w, "Invalid window duration", http.StatusBadRequest)
		return Request{}, time.Duration(0), err
	}

	return req, window, nil
}
