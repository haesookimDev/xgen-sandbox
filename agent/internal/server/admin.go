package server

import (
	"net/http"
	"strconv"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/audit"
)

// handleAdminSummary returns a high-level dashboard summary.
func (s *Server) handleAdminSummary(w http.ResponseWriter, r *http.Request) {
	sandboxes := s.sandboxMgr.List()

	byStatus := make(map[string]int)
	byTemplate := make(map[string]int)
	for _, sbx := range sandboxes {
		byStatus[string(sbx.Status)]++
		byTemplate[sbx.Template]++
	}

	// Read warm pool status. Keys are pool identifiers ("template" or
	// "template/caps"), preserved as-is so admin UI can group by template.
	warmPoolInfo := make(map[string]v1.WarmPoolInfo)
	for key, detail := range s.warmPool.Status() {
		warmPoolInfo[key] = v1.WarmPoolInfo{
			Available: detail.Available,
			Target:    detail.Target,
		}
	}

	writeJSON(w, http.StatusOK, v1.AdminSummaryResponse{
		ActiveSandboxes:     len(sandboxes),
		WarmPool:            warmPoolInfo,
		SandboxesByStatus:   byStatus,
		SandboxesByTemplate: byTemplate,
	})
}

// handleAdminMetrics returns a JSON snapshot of key Prometheus metrics.
func (s *Server) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	resp := v1.AdminMetricsResponse{
		ActiveSandboxes: gaugeValue(sandboxesActive),
	}
	writeJSON(w, http.StatusOK, resp)
}

// gaugeValue reads the current value from a Prometheus Gauge.
func gaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	return 0
}

// handleAdminAuditLogs returns paginated audit log entries.
func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	params := audit.QueryParams{
		Limit:   parseIntParam(r, "limit", 50),
		Offset:  parseIntParam(r, "offset", 0),
		Action:  r.URL.Query().Get("action"),
		Subject: r.URL.Query().Get("subject"),
	}

	result := s.auditStore.Query(params)

	entries := make([]v1.AuditEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = v1.AuditEntry{
			Timestamp: e.Timestamp,
			Action:    e.Action,
			Subject:   e.Subject,
			Role:      e.Role,
			Status:    e.Status,
			RemoteIP:  e.RemoteIP,
			SandboxID: e.SandboxID,
		}
	}

	writeJSON(w, http.StatusOK, v1.AdminAuditLogsResponse{
		Entries: entries,
		Total:   result.Total,
	})
}

// handleAdminWarmPool returns detailed warm pool state per
// (template, capability-set) pair.
func (s *Server) handleAdminWarmPool(w http.ResponseWriter, r *http.Request) {
	status := s.warmPool.Status()
	pools := make([]v1.WarmPoolDetail, 0, len(status))
	for _, detail := range status {
		pools = append(pools, v1.WarmPoolDetail{
			Template:     detail.Template,
			Capabilities: detail.Capabilities,
			Available:    detail.Available,
			Target:       detail.Target,
		})
	}

	writeJSON(w, http.StatusOK, v1.AdminWarmPoolResponse{
		Pools: pools,
	})
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
