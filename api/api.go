package api

import (
	"encoding/json"
	"net/http"

	"tier.run/api/types"
	"tier.run/client/tier"
	"tier.run/trweb"
)

type Server struct {
	Logf func(format string, args ...any)
	c    *tier.Client
}

func New(c *tier.Client) *Server {
	return &Server{c: c}
}

func (s *Server) logf(format string, args ...interface{}) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := s.serve(w, r)
	if err != nil {
		s.logf("error: %v", err)
		// continue processing error below
	}

	if trweb.WriteError(w, err) {
		return
	}
	// NOTE: if other errors should be coerced into nicer errors, this is
	// the place to do it.
	if err != nil {
		trweb.WriteError(w, trweb.InternalError)
		return
	}
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/v1/limits":
		return s.serveUsage(w, r)
	default:
		return trweb.NotFound
	}
}

func (s *Server) serveUsage(w http.ResponseWriter, r *http.Request) error {
	org := r.URL.Query().Get("org")
	if org == "" {
		return trweb.InvalidRequest
	}

	usage, err := s.c.LookupUsage(r.Context(), org)
	if err != nil {
		return err
	}

	var rr types.UsageResponse
	rr.Org = org
	for _, u := range usage {
		rr.Usage = append(rr.Usage, types.UsageUsageResponse(u))
	}
	return httpJSON(w, rr)
}

func httpJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
