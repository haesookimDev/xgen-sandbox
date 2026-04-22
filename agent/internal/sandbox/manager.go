package sandbox

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	v1 "github.com/xgen-sandbox/agent/api/v1"
)

// Sandbox represents a managed sandbox instance.
type Sandbox struct {
	ID          string
	Status      v1.SandboxStatus
	Template    string
	Ports       []int
	GUI         bool
	Env          map[string]string
	Metadata     map[string]string
	Capabilities []string
	CreatedAt    time.Time
	ExpiresAt   time.Time
	PodIP       string
}

// Manager tracks sandbox state and lifecycle.
type Manager struct {
	mu        sync.RWMutex
	sandboxes map[string]*Sandbox
}

// NewManager creates a new sandbox manager.
func NewManager() *Manager {
	return &Manager{
		sandboxes: make(map[string]*Sandbox),
	}
}

// Create registers a new sandbox and returns its ID.
func (m *Manager) Create(template string, timeout time.Duration, ports []int, gui bool, env, metadata map[string]string, capabilities []string) *Sandbox {
	id := generateID()
	now := time.Now()

	sbx := &Sandbox{
		ID:           id,
		Status:       v1.StatusStarting,
		Template:     template,
		Ports:        ports,
		GUI:          gui,
		Env:          env,
		Metadata:     metadata,
		Capabilities: capabilities,
		CreatedAt:    now,
		ExpiresAt:    now.Add(timeout),
	}

	m.mu.Lock()
	m.sandboxes[id] = sbx
	m.mu.Unlock()

	return sbx
}

// Get returns a sandbox by ID.
func (m *Manager) Get(id string) (*Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sbx, ok := m.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return sbx, nil
}

// List returns all sandboxes.
func (m *Manager) List() []*Sandbox {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Sandbox, 0, len(m.sandboxes))
	for _, sbx := range m.sandboxes {
		result = append(result, sbx)
	}
	return result
}

// SetStatus updates the status of a sandbox.
func (m *Manager) SetStatus(id string, status v1.SandboxStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sbx, ok := m.sandboxes[id]; ok {
		sbx.Status = status
	}
}

// SetPodIP updates the pod IP for a sandbox.
func (m *Manager) SetPodIP(id, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sbx, ok := m.sandboxes[id]; ok {
		sbx.PodIP = ip
	}
}

// Remove deletes a sandbox from tracking.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sandboxes, id)
}

// ExtendTimeout extends the expiration time.
func (m *Manager) ExtendTimeout(id string, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sbx, ok := m.sandboxes[id]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	sbx.ExpiresAt = time.Now().Add(duration)
	return nil
}

// GetExpired returns IDs of sandboxes past their expiration time.
func (m *Manager) GetExpired() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	var expired []string
	for id, sbx := range m.sandboxes {
		if now.After(sbx.ExpiresAt) && sbx.Status != v1.StatusStopped {
			expired = append(expired, id)
		}
	}
	return expired
}

// Recover re-registers a sandbox that was found in K8s after an agent restart.
//
// The caller is responsible for reading state from pod annotations and
// passing it in; this method is storage-only and does not consult K8s.
// A zero createdAt or expiresAt signals "unknown" — caller should have
// substituted a fallback (e.g. now + DefaultTimeout) before invocation,
// because Manager does not hold config.
func (m *Manager) Recover(
	id, template, podIP string,
	ports []int,
	gui bool,
	env, metadata map[string]string,
	capabilities []string,
	createdAt, expiresAt time.Time,
	ready bool,
) {
	status := v1.StatusStarting
	if ready {
		status = v1.StatusRunning
	}

	now := time.Now()
	if createdAt.IsZero() {
		createdAt = now
	}
	if expiresAt.IsZero() {
		// Caller failed to provide one — give the sandbox a short grace
		// period so it does not get reaped immediately.
		expiresAt = now.Add(time.Hour)
	}

	sbx := &Sandbox{
		ID:           id,
		Status:       status,
		Template:     template,
		Ports:        ports,
		GUI:          gui,
		Env:          env,
		Metadata:     metadata,
		PodIP:        podIP,
		Capabilities: capabilities,
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}

	m.mu.Lock()
	m.sandboxes[id] = sbx
	m.mu.Unlock()
}

func generateID() string {
	return uuid.New().String()[:12]
}
