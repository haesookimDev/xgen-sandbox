package k8s

import (
	"context"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestWarmPool(size int) (*WarmPool, *PodManager) {
	client := fake.NewSimpleClientset()
	pm := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)
	wp := NewWarmPool(pm, size, nil, nil)
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
	id := wp.Claim("base", nil)
	if id != "" {
		t.Errorf("expected empty string from empty pool, got %q", id)
	}
}

func TestWarmPool_MarkReadyAndClaim(t *testing.T) {
	wp, pm := newTestWarmPool(2)

	registerReady(pm, "warm-abc")
	wp.MarkReady("warm-abc", "base")

	id := wp.Claim("base", nil)
	if id != "warm-abc" {
		t.Errorf("expected warm-abc, got %q", id)
	}

	// Pool should be empty now
	id = wp.Claim("base", nil)
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

	if id := wp.Claim("base", nil); id != "first" {
		t.Errorf("expected first, got %q", id)
	}
	if id := wp.Claim("base", nil); id != "second" {
		t.Errorf("expected second, got %q", id)
	}
	if id := wp.Claim("base", nil); id != "third" {
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
	wp.Claim("base", nil)
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

	if id := wp.Claim("nodejs", nil); id != "node-1" {
		t.Errorf("expected node-1, got %q", id)
	}
	if id := wp.Claim("python", nil); id != "py-1" {
		t.Errorf("expected py-1, got %q", id)
	}

	// Claiming wrong template returns empty
	if id := wp.Claim("go", nil); id != "" {
		t.Errorf("expected empty for unclaimed template, got %q", id)
	}
}

func TestPoolKeyFor_NormalizesCapabilities(t *testing.T) {
	cases := []struct {
		template string
		caps     []string
		want     string
	}{
		{"base", nil, "base"},
		{"base", []string{}, "base"},
		{"base", []string{""}, "base"},
		{"base", []string{"sudo"}, "base/sudo"},
		// Order and duplicates should not affect the key.
		{"nodejs", []string{"git-ssh", "sudo"}, "nodejs/git-ssh,sudo"},
		{"nodejs", []string{"sudo", "git-ssh"}, "nodejs/git-ssh,sudo"},
		{"nodejs", []string{"sudo", "sudo"}, "nodejs/sudo"},
	}
	for _, tc := range cases {
		got := PoolKeyFor(tc.template, tc.caps)
		if got != tc.want {
			t.Errorf("PoolKeyFor(%q, %v) = %q, want %q", tc.template, tc.caps, got, tc.want)
		}
	}
}

func TestWarmPool_ClaimMatchesExactCapabilitySet(t *testing.T) {
	// A sudo request must only draw from the base/sudo pool, never from
	// the vanilla base pool. Regression test for the previous
	// len(caps)==0 guard in provisionSandboxPod.
	wp, pm := newTestWarmPool(2)

	registerReady(pm, "warm-plain")
	registerReady(pm, "warm-sudo")
	wp.MarkReady("warm-plain", PoolKeyFor("base", nil))
	wp.MarkReady("warm-sudo", PoolKeyFor("base", []string{"sudo"}))

	if id := wp.Claim("base", []string{"sudo"}); id != "warm-sudo" {
		t.Errorf("sudo claim: expected warm-sudo, got %q", id)
	}
	if id := wp.Claim("base", nil); id != "warm-plain" {
		t.Errorf("plain claim: expected warm-plain, got %q", id)
	}
	if id := wp.Claim("base", []string{"git-ssh"}); id != "" {
		t.Errorf("git-ssh claim with no matching pool: expected empty, got %q", id)
	}
}

func TestWarmPool_ClaimCapabilityOrderIndependent(t *testing.T) {
	// MarkReady uses the caller-computed key; Claim normalises caps so
	// callers can pass them in any order.
	wp, pm := newTestWarmPool(2)
	registerReady(pm, "multi")
	wp.MarkReady("multi", PoolKeyFor("base", []string{"sudo", "git-ssh"}))

	if id := wp.Claim("base", []string{"git-ssh", "sudo"}); id != "multi" {
		t.Errorf("reversed-order claim: expected multi, got %q", id)
	}
}

func TestWarmPool_Start_CreatesPodsForAllConfiguredCapsets(t *testing.T) {
	// With capabilities=["sudo"] the pool should issue CreatePod for
	// (base, nodejs, python) × (none, sudo) = 6 pods. Pods join the
	// tracked pool only on watcher onReady; here we just verify the
	// CreatePod dispatch by counting what hit the fake K8s client.
	client := fake.NewSimpleClientset()
	pm := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)
	wp := NewWarmPool(pm, 1, nil, []string{"sudo"})

	wp.Start(context.Background())

	pods, err := client.CoreV1().Pods("test-ns").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pods.Items) != 6 {
		t.Fatalf("expected 6 pods across (base,nodejs,python) × (none,sudo), got %d", len(pods.Items))
	}

	// Check the (template, caps) distribution — every configured spec
	// must appear exactly once.
	type key struct{ template, caps string }
	counts := map[key]int{}
	for _, p := range pods.Items {
		tmpl := p.Labels["xgen.io/template"]
		var caps []string
		for k := range p.Labels {
			if strings.HasPrefix(k, "xgen.io/cap-") {
				caps = append(caps, strings.TrimPrefix(k, "xgen.io/cap-"))
			}
		}
		sort.Strings(caps)
		counts[key{tmpl, strings.Join(caps, ",")}]++
	}
	want := map[key]int{
		{"base", ""}:      1,
		{"base", "sudo"}:  1,
		{"nodejs", ""}:    1,
		{"nodejs", "sudo"}: 1,
		{"python", ""}:    1,
		{"python", "sudo"}: 1,
	}
	for k, v := range want {
		if counts[k] != v {
			t.Errorf("pool %+v: expected %d pods, got %d", k, v, counts[k])
		}
	}
}
