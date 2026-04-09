package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/audit"
	"github.com/xgen-sandbox/agent/internal/auth"
	"github.com/xgen-sandbox/agent/internal/config"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/proxy"
	"github.com/xgen-sandbox/agent/internal/sandbox"
)

// Server is the main HTTP server for the agent.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	auth       *auth.Authenticator
	sandboxMgr *sandbox.Manager
	podMgr     *k8spkg.PodManager
	warmPool   *k8spkg.WarmPool
	wsProxy    *proxy.WSProxy
	router     *proxy.Router
	auditStore *audit.Store
}

// NewServer creates a new HTTP server.
func NewServer(
	cfg *config.Config,
	logger *slog.Logger,
	authenticator *auth.Authenticator,
	sandboxMgr *sandbox.Manager,
	podMgr *k8spkg.PodManager,
	warmPool *k8spkg.WarmPool,
	wsProxy *proxy.WSProxy,
	previewRouter *proxy.Router,
	auditStore *audit.Store,
) *Server {
	return &Server{
		cfg:        cfg,
		logger:     logger,
		auth:       authenticator,
		sandboxMgr: sandboxMgr,
		podMgr:     podMgr,
		warmPool:   warmPool,
		wsProxy:    wsProxy,
		router:     previewRouter,
		auditStore: auditStore,
	}
}

// Handler returns the configured HTTP handler.
// It routes preview domain requests to the preview router and everything else to the API router.
func (s *Server) Handler() http.Handler {
	apiRouter := s.apiHandler()
	previewHandler := s.router.Handler()
	domainHost := s.router.DomainHost()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}

		if strings.HasSuffix(host, "."+domainHost) {
			previewHandler.ServeHTTP(w, r)
			return
		}

		apiRouter.ServeHTTP(w, r)
	})
}

// apiHandler returns the chi router for API endpoints.
func (s *Server) apiHandler() http.Handler {
	r := chi.NewRouter()

	r.Use(structuredLogger(s.logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(metricsMiddleware)
	// TODO(#24): Add OpenTelemetry tracing middleware here
	// r.Use(otelchi.Middleware("xgen-agent"))

	// Health check and metrics (no auth)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Handle("/metrics", promhttp.Handler())

	// Auth endpoint (no auth required)
	r.Post("/api/v1/auth/token", s.handleAuthToken)

	// Protected API routes
	r.Group(func(r chi.Router) {
		r.Use(RateLimitMiddleware(s.cfg.RateLimitPerMinute))
		r.Use(s.auth.Middleware)
		r.Use(auditLog(s.logger, s.auditStore))

		r.With(auth.RequirePermission(auth.PermSandboxCreate)).
			Post("/api/v1/sandboxes", s.handleCreateSandbox)
		r.With(auth.RequirePermission(auth.PermSandboxRead)).
			Get("/api/v1/sandboxes", s.handleListSandboxes)
		r.With(auth.RequirePermission(auth.PermSandboxRead)).
			Get("/api/v1/sandboxes/{id}", s.handleGetSandbox)
		r.With(auth.RequirePermission(auth.PermSandboxDelete)).
			Delete("/api/v1/sandboxes/{id}", s.handleDeleteSandbox)
		r.With(auth.RequirePermission(auth.PermSandboxWrite)).
			Post("/api/v1/sandboxes/{id}/keepalive", s.handleKeepalive)
		r.With(auth.RequirePermission(auth.PermSandboxExec)).
			Post("/api/v1/sandboxes/{id}/exec", s.handleExec)
		r.With(auth.RequirePermission(auth.PermSandboxExec)).
			Get("/api/v1/sandboxes/{id}/ws", s.handleWS)
		r.With(auth.RequirePermission(auth.PermSandboxRead)).
			Get("/api/v1/sandboxes/{id}/services", s.handleListServices)

		// Admin-only routes
		r.Route("/api/v1/admin", func(r chi.Router) {
			r.Use(auth.RequireRole(auth.RoleAdmin))
			r.Get("/summary", s.handleAdminSummary)
			r.Get("/metrics", s.handleAdminMetrics)
			r.Get("/audit-logs", s.handleAdminAuditLogs)
			r.Get("/warm-pool", s.handleAdminWarmPool)
		})
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
		AuthFailuresTotal.WithLabelValues("invalid_api_key").Inc()
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	AuthTokenGeneratedTotal.Inc()
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

	// Validate ports
	seen := make(map[int]bool)
	for _, port := range req.Ports {
		if port < 1 || port > 65535 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid port: %d (must be 1-65535)", port))
			return
		}
		if seen[port] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("duplicate port: %d", port))
			return
		}
		seen[port] = true
	}

	// Validate metadata size
	if len(req.Metadata) > 20 {
		writeError(w, http.StatusBadRequest, "too many metadata entries (max 20)")
		return
	}
	for k, v := range req.Metadata {
		if len(k) > 128 || len(v) > 1024 {
			writeError(w, http.StatusBadRequest, "metadata key max 128 chars, value max 1024 chars")
			return
		}
	}

	timeout := s.cfg.DefaultTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
		if timeout > s.cfg.MaxTimeout {
			timeout = s.cfg.MaxTimeout
		}
	}

	sbx := s.sandboxMgr.Create(req.Template, timeout, req.Ports, req.GUI, req.Env, req.Metadata)
	sandboxCreateTotal.Inc()
	sandboxesActive.Inc()
	SandboxesByTemplate.WithLabelValues(req.Template).Inc()
	SandboxesByStatus.WithLabelValues(string(v1.StatusStarting)).Inc()

	// Try warm pool first, fall back to creating a new pod
	podCreateStart := time.Now()
	if warmID := s.warmPool.Claim(req.Template); warmID != "" {
		WarmPoolClaimsTotal.Inc()
		WarmPoolAvailable.WithLabelValues(req.Template).Dec()
		// Reuse warm pod: remap the warm pod's info to our new sandbox ID
		if info, ok := s.podMgr.GetPodInfo(warmID); ok {
			s.podMgr.RemapPod(warmID, sbx.ID)
			s.sandboxMgr.SetPodIP(sbx.ID, info.PodIP)
			s.sandboxMgr.SetStatus(sbx.ID, "running")
			SandboxPodCreateDuration.Observe(time.Since(podCreateStart).Seconds())
			log.Printf("claimed warm pod %s -> sandbox %s", warmID, sbx.ID)
			go s.warmPool.Replenish(context.Background(), req.Template)
		}
	} else {
		var rs *k8spkg.ResourceSpec
		if req.Resources != nil {
			rs = &k8spkg.ResourceSpec{CPU: req.Resources.CPU, Memory: req.Resources.Memory}
		}
		if err := s.podMgr.CreatePod(r.Context(), sbx.ID, req.Template, req.Env, req.Ports, req.GUI, rs); err != nil {
			s.sandboxMgr.Remove(sbx.ID)
			sandboxesActive.Dec()
			SandboxesByTemplate.WithLabelValues(req.Template).Dec()
			SandboxesByStatus.WithLabelValues(string(v1.StatusStarting)).Dec()
			log.Printf("create pod error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create sandbox")
			return
		}
		SandboxPodCreateDuration.Observe(time.Since(podCreateStart).Seconds())
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

	if req.GUI {
		vncURL := s.router.PreviewURL(sbx.ID, 6080)
		resp.VncURL = &vncURL
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
		if !apierrors.IsNotFound(err) {
			log.Printf("delete pod error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to delete sandbox pod")
			return
		}
		// Pod already gone — proceed with cleanup.
	}

	// Get sandbox info before removal for metrics
	if sbx, err := s.sandboxMgr.Get(id); err == nil {
		SandboxesByTemplate.WithLabelValues(sbx.Template).Dec()
		SandboxesByStatus.WithLabelValues(string(sbx.Status)).Dec()
	}

	s.wsProxy.DisconnectSidecar(id)
	s.sandboxMgr.SetStatus(id, v1.StatusStopped)
	s.sandboxMgr.Remove(id)
	sandboxDeleteTotal.Inc()
	sandboxesActive.Dec()

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
	if _, err := s.sandboxMgr.Get(id); err != nil {
		writeError(w, http.StatusNotFound, "sandbox not found")
		return
	}

	var req v1.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	timeout := 30 * time.Second
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	result, err := s.wsProxy.ExecSync(r.Context(), id, req.Command, req.Args, req.Env, req.Cwd, timeout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, v1.ExecResponse{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	})
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

	if sbx.GUI {
		vncURL := s.router.PreviewURL(sbx.ID, 6080)
		resp.VncURL = &vncURL
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
