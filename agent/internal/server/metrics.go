package server

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests by method, path, and status.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "xgen",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})

	sandboxesActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "sandboxes_active",
		Help:      "Number of active sandboxes.",
	})

	sandboxCreateTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "sandbox_create_total",
		Help:      "Total sandboxes created.",
	})

	sandboxDeleteTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "sandbox_delete_total",
		Help:      "Total sandboxes deleted.",
	})

	// Warm Pool metrics
	WarmPoolAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "warm_pool_available",
		Help:      "Number of available warm pods per template.",
	}, []string{"template"})

	WarmPoolTarget = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "warm_pool_target",
		Help:      "Target warm pool size per template.",
	}, []string{"template"})

	WarmPoolClaimsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "warm_pool_claims_total",
		Help:      "Total warm pool pod claims.",
	})

	WarmPoolReplenishFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "warm_pool_replenish_failures_total",
		Help:      "Total warm pool replenish failures.",
	})

	// WebSocket metrics
	WsConnectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "ws_connections_active",
		Help:      "Number of active WebSocket connections.",
	})

	// Auth metrics
	AuthTokenGeneratedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "auth_token_generated_total",
		Help:      "Total JWT tokens generated.",
	})

	AuthFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "xgen",
		Name:      "auth_failures_total",
		Help:      "Total authentication failures by reason.",
	}, []string{"reason"})

	// Pod lifecycle metrics
	SandboxPodCreateDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "xgen",
		Name:      "sandbox_pod_create_duration_seconds",
		Help:      "Duration of sandbox pod creation in seconds.",
		Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	})

	// Sandbox breakdown metrics
	SandboxesByTemplate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "sandboxes_by_template",
		Help:      "Active sandboxes by template.",
	}, []string{"template"})

	SandboxesByStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "xgen",
		Name:      "sandboxes_by_status",
		Help:      "Sandboxes by status.",
	}, []string{"status"})
)

// metricsMiddleware records HTTP request metrics.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)

		// Use the chi route pattern for grouping, fall back to raw path
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = r.URL.Path
		}

		duration := time.Since(start).Seconds()
		httpRequestsTotal.WithLabelValues(r.Method, routePattern, strconv.Itoa(ww.status)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, routePattern).Observe(duration)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so WebSocket upgrades work through middleware.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}
