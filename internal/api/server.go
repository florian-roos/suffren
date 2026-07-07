package api

import (
	"net/http"

	"github.com/florian-roos/suffren/internal/ratelimiter"
)

type CheckRequest struct {
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
	Error     error  `json:"error,omitempty"`
}

type Server struct {
	router  *http.ServeMux
	limiter *ratelimiter.Limiter
}

func NewServer(limiter *ratelimiter.Limiter) *Server {
	s := &Server{
		router:  http.NewServeMux(),
		limiter: limiter,
	}

	s.router.HandleFunc("POST /check", s.handleCheck)
	s.router.HandleFunc("GET /status", s.handleStatus)

	return s
}

// starts the API HTTP server.
func (s *Server) Start(addr string) {}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request)  {}
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {}
