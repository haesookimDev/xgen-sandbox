package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestWarmPool(size int) (*WarmPool, *PodManager) {
	client := fake.NewSimpleClientset()
	pm := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)
	wp := NewWarmPool(pm, size)
	return wp, pm
}

// registerReady adds a PodInfo entry marked Ready so that Claim can find it.
func registerReady(pm *PodManager, sandboxID string) {
	pm.mu.Lock()
	pm.pods[sandboxID] = &PodInfo{
		SandboxID: sandboxID,
		PodName:   "sbx-" + sandboxID,
		PodIP:     "10.0.0.1",
		Phase:     corev1.PodRunning,
		Ready:     true,
	}
	pm.mu.Unlock()
}

func TestWarmPool_ClaimEmpty(t *testing.T) {
	wp, _ := newTestWarmPool(2)
	id := wp.Claim("base")
	if id != "" {
		t.Errorf("expected empty string from empty pool, got %q", id)
	}
}

func TestWarmPool_MarkReadyAndClaim(t *testing.T) {
	wp, pm := newTestWarmPool(2)

	registerReady(pm, "warm-abc")
	wp.MarkReady("warm-abc", "base")

	id := wp.Claim("base")
	if id != "warm-abc" {
		t.Errorf("expected warm-abc, got %q", id)
	}

	// Pool should be empty now
	id = wp.Claim("base")
	if id != "" {
		t.Errorf("expected empty string after claim, got %q", id)
	}
}

func TestWarmPool_ClaimFIFO(t *testing.T) {
	wp, pm := newTestWarmPool(3)

	for _, id := range []string{"first", "second", "third"} {
		registerReady(pm, id)
	}
	wp.MarkReady("first", "base")
	wp.MarkReady("second", "base")
	wp.MarkReady("third", "base")

	if id := wp.Claim("base"); id != "first" {
		t.Errorf("expected first, got %q", id)
	}
	if id := wp.Claim("base"); id != "second" {
		t.Errorf("expected second, got %q", id)
	}
	if id := wp.Claim("base"); id != "third" {
		t.Errorf("expected third, got %q", id)
	}
}

func TestWarmPool_IsWarm(t *testing.T) {
	wp, pm := newTestWarmPool(2)

	registerReady(pm, "warm-1")
	wp.MarkReady("warm-1", "base")

	if !wp.IsWarm("warm-1") {
		t.Error("expected warm-1 to be warm")
	}
	if wp.IsWarm("unknown") {
		t.Error("expected unknown to not be warm")
	}

	// After claim, should no longer be warm
	wp.Claim("base")
	if wp.IsWarm("warm-1") {
		t.Error("expected warm-1 to not be warm after claim")
	}
}

func TestWarmPool_Size(t *testing.T) {
	wp, _ := newTestWarmPool(5)

	if s := wp.Size("base"); s != 0 {
		t.Errorf("expected size 0, got %d", s)
	}

	wp.MarkReady("a", "base")
	wp.MarkReady("b", "base")

	if s := wp.Size("base"); s != 2 {
		t.Errorf("expected size 2, got %d", s)
	}
}

func TestWarmPool_Start_SizeZero(t *testing.T) {
	wp, _ := newTestWarmPool(0)
	// Should be a no-op
	wp.Start(context.Background())
	if s := wp.Size("base"); s != 0 {
		t.Errorf("expected size 0 for size=0 pool, got %d", s)
	}
}

func TestWarmPool_ClaimDifferentTemplates(t *testing.T) {
	wp, pm := newTestWarmPool(2)

	registerReady(pm, "node-1")
	registerReady(pm, "py-1")
	wp.MarkReady("node-1", "nodejs")
	wp.MarkReady("py-1", "python")

	if id := wp.Claim("nodejs"); id != "node-1" {
		t.Errorf("expected node-1, got %q", id)
	}
	if id := wp.Claim("python"); id != "py-1" {
		t.Errorf("expected py-1, got %q", id)
	}

	// Claiming wrong template returns empty
	if id := wp.Claim("go"); id != "" {
		t.Errorf("expected empty for unclaimed template, got %q", id)
	}
}
