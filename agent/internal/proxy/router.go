package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
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

		// WebSocket upgrade requests need raw TCP proxying
		if isWebSocketUpgrade(req) {
			proxyWebSocket(w, req, target)
			return
		}

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

// isWebSocketUpgrade checks if the request is a WebSocket upgrade.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// proxyWebSocket proxies a WebSocket upgrade request via raw TCP tunneling.
func proxyWebSocket(w http.ResponseWriter, r *http.Request, target *url.URL) {
	backend, err := net.Dial("tcp", target.Host)
	if err != nil {
		log.Printf("ws proxy: dial backend %s: %v", target.Host, err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer backend.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	// Forward the original request to the backend
	r.URL = target
	r.Host = target.Host
	if err := r.Write(backend); err != nil {
		log.Printf("ws proxy: write request to backend: %v", err)
		http.Error(w, "backend write error", http.StatusBadGateway)
		return
	}

	client, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("ws proxy: hijack: %v", err)
		return
	}
	defer client.Close()

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(backend, client); done <- struct{}{} }()
	go func() { io.Copy(client, backend); done <- struct{}{} }()
	<-done
}
