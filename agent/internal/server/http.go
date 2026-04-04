package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/auth"
	"github.com/xgen-sandbox/agent/internal/config"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/proxy"
	"github.com/xgen-sandbox/agent/internal/sandbox"
)

// Server is the main HTTP server for the agent.
type Server struct {
	cfg        *config.Config
	auth       *auth.Authenticator
	sandboxMgr *sandbox.Manager
	podMgr     *k8spkg.PodManager
	wsProxy    *proxy.WSProxy
	router     *proxy.Router
}

// NewServer creates a new HTTP server.
func NewServer(
	cfg *config.Config,
	authenticator *auth.Authenticator,
	sandboxMgr *sandbox.Manager,
	podMgr *k8spkg.PodManager,
	wsProxy *proxy.WSProxy,
	previewRouter *proxy.Router,
) *Server {
	return &Server{
		cfg:        cfg,
		auth:       authenticator,
		sandboxMgr: sandboxMgr,
		podMgr:     podMgr,
		wsProxy:    wsProxy,
		router:     previewRouter,
	}
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health check (no auth)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Auth endpoint (no auth required)
	r.Post("/api/v1/auth/token", s.handleAuthToken)

	// Protected API routes
	r.Group(func(r chi.Router) {
		r.Use(s.auth.Middleware)

		r.Post("/api/v1/sandboxes", s.handleCreateSandbox)
		r.Get("/api/v1/sandboxes", s.handleListSandboxes)
		r.Get("/api/v1/sandboxes/{id}", s.handleGetSandbox)
		r.Delete("/api/v1/sandboxes/{id}", s.handleDeleteSandbox)
		r.Post("/api/v1/sandboxes/{id}/keepalive", s.handleKeepalive)
		r.Post("/api/v1/sandboxes/{id}/exec", s.handleExec)
		r.Get("/api/v1/sandboxes/{id}/ws", s.handleWS)
		r.Get("/api/v1/sandboxes/{id}/services", s.handleListServices)
	})

	return r
}

// --- Auth ---

func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	var req v1.AuthTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, expiresAt, err := s.auth.GenerateToken(req.APIKey)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, v1.AuthTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}

// --- Sandbox CRUD ---

func (s *Server) handleCreateSandbox(w http.ResponseWriter, r *http.Request) {
	var req v1.CreateSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Template == "" {
		req.Template = "base"
	}

	timeout := s.cfg.DefaultTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
		if timeout > s.cfg.MaxTimeout {
			timeout = s.cfg.MaxTimeout
		}
	}

	sbx := s.sandboxMgr.Create(req.Template, timeout, req.Ports, req.GUI, req.Env, req.Metadata)

	// Create K8s pod
	if err := s.podMgr.CreatePod(r.Context(), sbx.ID, req.Template, req.Env, req.Ports, req.GUI); err != nil {
		s.sandboxMgr.Remove(sbx.ID)
		log.Printf("create pod error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	// Build preview URLs
	previewURLs := make(map[int]string)
	for _, port := range req.Ports {
		previewURLs[port] = s.router.PreviewURL(sbx.ID, port)
	}

	resp := v1.SandboxResponse{
		ID:          sbx.ID,
		Status:      sbx.Status,
		Template:    sbx.Template,
		WsURL:       fmt.Sprintf("%s/api/v1/sandboxes/%s/ws", s.cfg.ExternalURL, sbx.ID),
		PreviewURLs: previewURLs,
		CreatedAt:   sbx.CreatedAt,
		ExpiresAt:   sbx.ExpiresAt,
		Metadata:    sbx.Metadata,
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes := s.sandboxMgr.List()
	result := make([]v1.SandboxResponse, 0, len(sandboxes))
	for _, sbx := range sandboxes {
		result = append(result, s.sandboxToResponse(sbx))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sbx, err := s.sandboxMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sandboxToResponse(sbx))
}

func (s *Server) handleDeleteSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.podMgr.DeletePod(r.Context(), id); err != nil {
		log.Printf("delete pod error: %v", err)
	}

	s.wsProxy.DisconnectSidecar(id)
	s.sandboxMgr.SetStatus(id, v1.StatusStopped)
	s.sandboxMgr.Remove(id)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKeepalive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.sandboxMgr.ExtendTimeout(id, s.cfg.DefaultTimeout); err != nil {
		writeError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Exec (REST convenience) ---

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sbx, err := s.sandboxMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sandbox not found")
		return
	}

	var req v1.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_ = sbx // exec via REST uses the WebSocket proxy internally
	// For Phase 1, we execute synchronously through the sidecar WS
	// This is a simplified implementation; full streaming goes through the WS endpoint

	writeError(w, http.StatusNotImplemented, "use WebSocket endpoint for exec in Phase 1")
}

// --- WebSocket ---

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.wsProxy.HandleClientWS(w, r, id)
}

// --- Services ---

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sbx, err := s.sandboxMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sandbox not found")
		return
	}

	services := make([]map[string]any, 0, len(sbx.Ports))
	for _, port := range sbx.Ports {
		services = append(services, map[string]any{
			"port":        port,
			"preview_url": s.router.PreviewURL(sbx.ID, port),
		})
	}
	writeJSON(w, http.StatusOK, services)
}

// --- Helpers ---

func (s *Server) sandboxToResponse(sbx *sandbox.Sandbox) v1.SandboxResponse {
	previewURLs := make(map[int]string)
	for _, port := range sbx.Ports {
		previewURLs[port] = s.router.PreviewURL(sbx.ID, port)
	}

	resp := v1.SandboxResponse{
		ID:          sbx.ID,
		Status:      sbx.Status,
		Template:    sbx.Template,
		WsURL:       fmt.Sprintf("%s/api/v1/sandboxes/%s/ws", s.cfg.ExternalURL, sbx.ID),
		PreviewURLs: previewURLs,
		CreatedAt:   sbx.CreatedAt,
		ExpiresAt:   sbx.ExpiresAt,
		Metadata:    sbx.Metadata,
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, v1.ErrorResponse{Error: message})
}
