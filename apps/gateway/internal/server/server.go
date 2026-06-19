package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/madfam-org/coupler/apps/gateway/internal/auth"
	"github.com/madfam-org/coupler/apps/gateway/internal/executor"
	"github.com/madfam-org/coupler/apps/gateway/internal/registry"
)

const version = "0.2.0"

type Server struct {
	reg      *registry.Registry
	verifier *auth.Verifier
	exec     *executor.Executor
}

func New(reg *registry.Registry, verifier *auth.Verifier) *Server {
	return &Server{reg: reg, verifier: verifier, exec: executor.New()}
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
		"phase":   "2",
	})
}

func (s *Server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	tools := s.reg.List()
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "count": len(tools)})
}

func (s *Server) handleSearchTools(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	matched := s.reg.Search(q)
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "tools": matched, "count": len(matched)})
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
			"dry_run":   true,
			"tool":      tool.Name,
			"connector": tool.Connector,
			"plan": map[string]any{
				"arguments":     req.Arguments,
				"connection_id": req.ConnectionID,
				"auth":          "janua_token_delegation",
			},
		})
		return
	}

	claims, hasClaims := auth.ClaimsFromContext(r.Context())
	if !hasClaims || claims.Sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user_jwt_required"})
		return
	}

	result, err := s.exec.Execute(r.Context(), tool, executor.Request{
		Tool:         req.Tool,
		Arguments:    req.Arguments,
		ConnectionID: req.ConnectionID,
		ActingUserID: claims.Sub,
		UserJWT:      auth.UserJWTFromContext(r.Context()),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "execute_failed",
			"tool":    tool.Name,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dry_run": false,
		"tool":    result.Tool,
		"connector": result.Connector,
		"result":  result.Output,
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
	candidates := []string{"connectors", "../../connectors", "../../../connectors"}
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
