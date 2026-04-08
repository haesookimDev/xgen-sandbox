package audit

import (
	"strings"
	"sync"
	"time"
)

// Entry represents a single audit log entry.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Subject   string    `json:"subject"`
	Role      string    `json:"role"`
	Status    int       `json:"status"`
	RemoteIP  string    `json:"remote_ip"`
	SandboxID string    `json:"sandbox_id,omitempty"`
}

// QueryParams defines filters for querying audit entries.
type QueryParams struct {
	Limit   int
	Offset  int
	Action  string
	Subject string
}

// QueryResult holds the result of an audit log query.
type QueryResult struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
}

// Store is a thread-safe, in-memory ring buffer for audit log entries.
type Store struct {
	mu      sync.RWMutex
	entries []Entry
	maxSize int
}

// NewStore creates a new audit store with the given capacity.
func NewStore(maxSize int) *Store {
	return &Store{
		entries: make([]Entry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add appends a new audit entry. If the buffer is full, the oldest entry is evicted.
func (s *Store) Add(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.entries) >= s.maxSize {
		s.entries = s.entries[1:]
	}
	s.entries = append(s.entries, entry)
}

// Query returns audit entries matching the given filters with pagination.
func (s *Store) Query(params QueryParams) QueryResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter entries (iterate in reverse for newest-first)
	var filtered []Entry
	for i := len(s.entries) - 1; i >= 0; i-- {
		e := s.entries[i]
		if params.Action != "" && !strings.Contains(e.Action, params.Action) {
			continue
		}
		if params.Subject != "" && e.Subject != params.Subject {
			continue
		}
		filtered = append(filtered, e)
	}

	total := len(filtered)

	// Apply pagination
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Offset > total {
		return QueryResult{Entries: []Entry{}, Total: total}
	}

	end := params.Offset + params.Limit
	if end > total {
		end = total
	}

	return QueryResult{
		Entries: filtered[params.Offset:end],
		Total:   total,
	}
}

// Size returns the current number of entries in the store.
func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}
