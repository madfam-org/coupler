package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/madfam-org/coupler/apps/gateway/internal/auth"
	"github.com/madfam-org/coupler/apps/gateway/internal/registry"
)

const version = "0.1.0-phase0"

type Server struct {
	reg      *registry.Registry
	verifier *auth.Verifier
}

func New(reg *registry.Registry, verifier *auth.Verifier) *Server {
	return &Server{reg: reg, verifier: verifier}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/tools", s.handleListTools)
	mux.HandleFunc("GET /v1/tools/search", s.handleSearchTools)
	mux.HandleFunc("POST /v1/tools/execute", s.handleExecute)
	return s.verifier.Middleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "coupler-gateway",
		"version": version,
		"phase":   "0",
	})
}

func (s *Server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tools": s.reg.List(),
		"count": len(s.reg.List()),
	})
}

func (s *Server) handleSearchTools(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	writeJSON(w, http.StatusOK, map[string]any{
		"query": q,
		"tools": s.reg.Search(q),
		"count": len(s.reg.Search(q)),
	})
}

type executeRequest struct {
	Tool         string         `json:"tool"`
	Arguments    map[string]any `json:"arguments"`
	DryRun       bool           `json:"dry_run"`
	ConnectionID string         `json:"connection_id"`
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool_required"})
		return
	}

	tool, ok := s.reg.Get(req.Tool)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tool_not_found", "tool": req.Tool})
		return
	}

	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"dry_run": true,
			"tool":    tool.Name,
			"connector": tool.Connector,
			"plan": map[string]any{
				"arguments":     req.Arguments,
				"connection_id": req.ConnectionID,
				"auth":          "janua_token_delegation",
			},
			"message": "Dry run OK — live execute blocked until Janua ConnectedAccount delegation (P1)",
		})
		return
	}

	// Live execute requires Janua P1
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error":   "live_execute_not_available",
		"tool":    tool.Name,
		"phase":   "0",
		"blocker": "janua_connected_account_delegation",
		"hint":    "Set dry_run=true or wait for Janua P1",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ConnectorsDir() string {
	if v := os.Getenv("COUPLER_CONNECTORS_DIR"); v != "" {
		return v
	}
	// monorepo default: repo root connectors/
	candidates := []string{
		"connectors",
		"../../connectors",
		"../../../connectors",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "connectors"
}

func AuthRequired() bool {
	return os.Getenv("COUPLER_AUTH_REQUIRED") == "true"
}
