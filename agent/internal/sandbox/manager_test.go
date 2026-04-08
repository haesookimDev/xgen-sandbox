package sandbox

import (
	"sync"
	"testing"
	"time"

	v1 "github.com/xgen-sandbox/agent/api/v1"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := NewManager()
	sbx := m.Create("nodejs", time.Hour, []int{3000}, false, nil, nil)

	if sbx.ID == "" {
		t.Fatal("expected non-empty sandbox ID")
	}
	if sbx.Status != v1.StatusStarting {
		t.Errorf("status: expected %q, got %q", v1.StatusStarting, sbx.Status)
	}
	if sbx.Template != "nodejs" {
		t.Errorf("template: expected nodejs, got %q", sbx.Template)
	}

	got, err := m.Get(sbx.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != sbx.ID {
		t.Errorf("Get() returned wrong sandbox")
	}
}

func TestManager_GetNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent sandbox")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()
	m.Create("nodejs", time.Hour, nil, false, nil, nil)
	m.Create("python", time.Hour, nil, false, nil, nil)
	m.Create("go", time.Hour, nil, false, nil, nil)

	list := m.List()
	if len(list) != 3 {
		t.Errorf("List: expected 3 sandboxes, got %d", len(list))
	}
}

func TestManager_ListEmpty(t *testing.T) {
	m := NewManager()
	list := m.List()
	if list == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

func TestManager_SetStatus(t *testing.T) {
	m := NewManager()
	sbx := m.Create("nodejs", time.Hour, nil, false, nil, nil)

	m.SetStatus(sbx.ID, v1.StatusRunning)

	got, _ := m.Get(sbx.ID)
	if got.Status != v1.StatusRunning {
		t.Errorf("status: expected %q, got %q", v1.StatusRunning, got.Status)
	}
}

func TestManager_SetPodIP(t *testing.T) {
	m := NewManager()
	sbx := m.Create("nodejs", time.Hour, nil, false, nil, nil)

	m.SetPodIP(sbx.ID, "10.0.0.5")

	got, _ := m.Get(sbx.ID)
	if got.PodIP != "10.0.0.5" {
		t.Errorf("PodIP: expected 10.0.0.5, got %q", got.PodIP)
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()
	sbx := m.Create("nodejs", time.Hour, nil, false, nil, nil)

	m.Remove(sbx.ID)

	_, err := m.Get(sbx.ID)
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestManager_ExtendTimeout(t *testing.T) {
	m := NewManager()
	sbx := m.Create("nodejs", time.Minute, nil, false, nil, nil)

	err := m.ExtendTimeout(sbx.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("ExtendTimeout() error: %v", err)
	}

	got, _ := m.Get(sbx.ID)
	// ExpiresAt should be roughly 2 hours from now
	expected := time.Now().Add(2 * time.Hour)
	diff := got.ExpiresAt.Sub(expected)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("ExpiresAt too far from expected: diff=%v", diff)
	}
}

func TestManager_ExtendTimeout_NotFound(t *testing.T) {
	m := NewManager()
	err := m.ExtendTimeout("nonexistent", time.Hour)
	if err == nil {
		t.Error("expected error for nonexistent sandbox")
	}
}

func TestManager_GetExpired(t *testing.T) {
	m := NewManager()

	// Create a sandbox that expires immediately
	expired := m.Create("nodejs", time.Millisecond, nil, false, nil, nil)
	// Create a sandbox with long timeout
	m.Create("python", time.Hour, nil, false, nil, nil)

	// Wait for the short-timeout sandbox to expire
	time.Sleep(5 * time.Millisecond)

	expiredIDs := m.GetExpired()
	if len(expiredIDs) != 1 {
		t.Fatalf("expected 1 expired sandbox, got %d", len(expiredIDs))
	}
	if expiredIDs[0] != expired.ID {
		t.Errorf("expected expired ID %q, got %q", expired.ID, expiredIDs[0])
	}
}

func TestManager_GetExpired_SkipsStopped(t *testing.T) {
	m := NewManager()

	sbx := m.Create("nodejs", 0, nil, false, nil, nil)
	m.SetStatus(sbx.ID, v1.StatusStopped)

	expiredIDs := m.GetExpired()
	if len(expiredIDs) != 0 {
		t.Errorf("expected 0 expired (stopped excluded), got %d", len(expiredIDs))
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// Spawn 100 goroutines doing concurrent operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sbx := m.Create("nodejs", time.Hour, nil, false, nil, nil)
			m.Get(sbx.ID)
			m.List()
			m.SetStatus(sbx.ID, v1.StatusRunning)
			m.SetPodIP(sbx.ID, "10.0.0.1")
			m.Remove(sbx.ID)
		}()
	}

	wg.Wait()

	// All sandboxes should be removed
	list := m.List()
	if len(list) != 0 {
		t.Errorf("expected 0 sandboxes after cleanup, got %d", len(list))
	}
}
