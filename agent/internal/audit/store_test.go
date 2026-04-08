package audit

import (
	"testing"
	"time"
)

func TestStore_AddAndQuery(t *testing.T) {
	store := NewStore(5)

	for i := 0; i < 3; i++ {
		store.Add(Entry{
			Timestamp: time.Now(),
			Action:    "POST /api/v1/sandboxes",
			Subject:   "default",
			Role:      "admin",
			Status:    201,
			RemoteIP:  "127.0.0.1",
		})
	}

	result := store.Query(QueryParams{Limit: 10})
	if result.Total != 3 {
		t.Errorf("expected total 3, got %d", result.Total)
	}
	if len(result.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result.Entries))
	}
}

func TestStore_RingBufferEviction(t *testing.T) {
	store := NewStore(3)

	for i := 0; i < 5; i++ {
		store.Add(Entry{
			Timestamp: time.Now(),
			Action:    "action",
			Subject:   "user",
			Status:    200 + i,
		})
	}

	if store.Size() != 3 {
		t.Errorf("expected size 3 after eviction, got %d", store.Size())
	}

	result := store.Query(QueryParams{Limit: 10})
	// Should have the last 3 entries (status 202, 203, 204)
	if result.Total != 3 {
		t.Errorf("expected total 3, got %d", result.Total)
	}
}

func TestStore_QueryFilters(t *testing.T) {
	store := NewStore(100)

	store.Add(Entry{Action: "POST /api/v1/sandboxes", Subject: "alice", Status: 201})
	store.Add(Entry{Action: "DELETE /api/v1/sandboxes/abc", Subject: "bob", Status: 204})
	store.Add(Entry{Action: "POST /api/v1/sandboxes", Subject: "alice", Status: 201})

	// Filter by action
	result := store.Query(QueryParams{Limit: 10, Action: "DELETE"})
	if result.Total != 1 {
		t.Errorf("expected 1 DELETE entry, got %d", result.Total)
	}

	// Filter by subject
	result = store.Query(QueryParams{Limit: 10, Subject: "alice"})
	if result.Total != 2 {
		t.Errorf("expected 2 entries for alice, got %d", result.Total)
	}
}

func TestStore_QueryPagination(t *testing.T) {
	store := NewStore(100)

	for i := 0; i < 10; i++ {
		store.Add(Entry{Action: "action", Subject: "user", Status: 200})
	}

	result := store.Query(QueryParams{Limit: 3, Offset: 0})
	if len(result.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result.Entries))
	}
	if result.Total != 10 {
		t.Errorf("expected total 10, got %d", result.Total)
	}

	result = store.Query(QueryParams{Limit: 3, Offset: 8})
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries at offset 8, got %d", len(result.Entries))
	}
}
