package proxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/xgen-sandbox/agent/internal/sandbox"
)

// Router provides dynamic reverse proxying to sandbox services.
type Router struct {
	previewDomain string
	sandboxMgr    *sandbox.Manager
}

// NewRouter creates a new sandbox service router.
func NewRouter(previewDomain string, sandboxMgr *sandbox.Manager) *Router {
	return &Router{
		previewDomain: previewDomain,
		sandboxMgr:    sandboxMgr,
	}
}

// Handler returns an http.Handler that routes preview requests to sandbox pods.
// Expected hostname format: sbx-{id}-{port}.preview.example.com
func (r *Router) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host := req.Host
		// Strip port from host if present
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// Check if this is a preview domain request
		if !strings.HasSuffix(host, "."+r.previewDomain) {
			http.Error(w, "not a preview domain", http.StatusNotFound)
			return
		}

		// Parse subdomain: sbx-{id}-{port}
		subdomain := strings.TrimSuffix(host, "."+r.previewDomain)
		sandboxID, port, err := parseSubdomain(subdomain)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sbx, err := r.sandboxMgr.Get(sandboxID)
		if err != nil {
			http.Error(w, "sandbox not found", http.StatusNotFound)
			return
		}

		if sbx.PodIP == "" {
			http.Error(w, "sandbox not ready", http.StatusServiceUnavailable)
			return
		}

		target, _ := url.Parse(fmt.Sprintf("http://%s:%s", sbx.PodIP, port))
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error for sandbox %s: %v", sandboxID, err)
			http.Error(w, "sandbox service unavailable", http.StatusBadGateway)
		}

		proxy.ServeHTTP(w, req)
	})
}

// PreviewURL generates the preview URL for a sandbox port.
func (r *Router) PreviewURL(sandboxID string, port int) string {
	return fmt.Sprintf("https://sbx-%s-%d.%s", sandboxID, port, r.previewDomain)
}

// parseSubdomain extracts sandbox ID and port from "sbx-{id}-{port}".
func parseSubdomain(subdomain string) (sandboxID string, port string, err error) {
	if !strings.HasPrefix(subdomain, "sbx-") {
		return "", "", fmt.Errorf("invalid subdomain format: %s", subdomain)
	}

	rest := strings.TrimPrefix(subdomain, "sbx-")
	// Last segment after '-' is the port
	lastDash := strings.LastIndex(rest, "-")
	if lastDash == -1 {
		return "", "", fmt.Errorf("invalid subdomain format: %s", subdomain)
	}

	sandboxID = rest[:lastDash]
	port = rest[lastDash+1:]
	return sandboxID, port, nil
}
